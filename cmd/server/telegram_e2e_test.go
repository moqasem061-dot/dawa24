package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/datasets"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	assistantPostgres "github.com/muhiya/dawa24-store/internal/modules/assistant/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	identityPostgres "github.com/muhiya/dawa24-store/internal/modules/identity/postgres"
	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/telegram"
	telegramHTTP "github.com/muhiya/dawa24-store/internal/modules/telegram/http"
	telegramPostgres "github.com/muhiya/dawa24-store/internal/modules/telegram/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The whole path, against a real database: n8n's HTTP call, the bridge's
// authentication, link confirmation, the RBAC resolver, the real assistant
// service and tool registry, the audit trail, and the notification outbox.
// Only the language model is scripted — it asks query_data for an order count, then answers.
//
// TEST_DATABASE_URL only: this writes fixture rows.

const e2eToken = "e2e-bridge-token-0123456789abcdef0123456789"

// scriptedModel calls query_data on its first round and answers on the next.
type scriptedModel struct {
	mu    sync.Mutex
	calls []gateway.ChatRequest
}

func (m *scriptedModel) Stream(_ context.Context, req gateway.ChatRequest) (<-chan gateway.StreamEvent, error) {
	m.mu.Lock()
	m.calls = append(m.calls, req)
	m.mu.Unlock()
	ch := make(chan gateway.StreamEvent, 2)
	last := req.Messages[len(req.Messages)-1]
	if last.Role == "tool" {
		ch <- gateway.StreamEvent{Delta: "لديك **0** طلبات."}
		ch <- gateway.StreamEvent{Done: true, Usage: &gateway.Usage{PromptTokens: 10, CompletionTokens: 5}}
	} else {
		ch <- gateway.StreamEvent{Done: true, ToolCalls: []gateway.ToolCall{{ID: "c1", Name: "query_data", Arguments: `{"dataset":"purchase_orders","metrics":["count"]}`}}}
	}
	close(ch)
	return ch, nil
}

func (m *scriptedModel) Invoke(context.Context, gateway.Request) (*gateway.Response, error) {
	return nil, errors.New("not scripted")
}
func (m *scriptedModel) Transcribe(context.Context, gateway.TranscribeRequest) (string, error) {
	return "", errors.New("not scripted")
}
func (m *scriptedModel) Capabilities(context.Context, gateway.Role) (gateway.ModelCapabilities, error) {
	return gateway.ModelCapabilities{}, errors.New("not scripted")
}
func (m *scriptedModel) Health(context.Context) error { return nil }
func (m *scriptedModel) Enabled() bool                { return false }

type e2e struct {
	t      *testing.T
	db     *database.DB
	ctx    context.Context
	srv    *httptest.Server
	tg     *telegram.Service
	grants *rbac.Resolver
	update int64
}

func (e *e2e) scalar(sql string, args ...any) int64 {
	e.t.Helper()
	var id int64
	if err := e.db.Pool().QueryRow(e.ctx, sql, args...).Scan(&id); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (e *e2e) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Pool().Exec(e.ctx, sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *e2e) post(path, body string) (int, map[string]any) {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+telegramHTTP.Prefix+path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+e2eToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// say sends a private text message as Telegram would and returns the reply text.
func (e *e2e) say(tgUser int64, text string) string {
	e.t.Helper()
	e.update++
	body, _ := json.Marshal(map[string]any{
		"update_id": e.update,
		"message": map[string]any{
			"message_id": e.update,
			"from":       map[string]any{"id": tgUser, "is_bot": false, "first_name": "Test", "username": fmt.Sprintf("u%d", tgUser)},
			"chat":       map[string]any{"id": tgUser, "type": "private"},
			"text":       text,
		},
	})
	code, out := e.post("/updates", string(body))
	if code != http.StatusOK {
		e.t.Fatalf("updates status %d: %v", code, out)
	}
	var b strings.Builder
	msgs, _ := out["messages"].([]any)
	for _, m := range msgs {
		mm := m.(map[string]any)
		if mm["chat_id"] != fmt.Sprint(tgUser) {
			e.t.Fatalf("reply addressed to chat %v, not %d", mm["chat_id"], tgUser)
		}
		b.WriteString(mm["text"].(string) + "\n")
	}
	return b.String()
}

func (e *e2e) link(userID, orgID, tgUser int64) {
	e.t.Helper()
	g, err := e.grants.Resolve(e.ctx, userID, orgID)
	if err != nil {
		e.t.Fatal(err)
	}
	code, err := e.tg.StartLink(e.ctx, authctx.FromGrant(g))
	if err != nil {
		e.t.Fatal(err)
	}
	e.say(tgUser, "/start "+code.DeepLink[strings.Index(code.DeepLink, "=")+1:])
	current, err := e.tg.CurrentLink(e.ctx, userID)
	if err != nil || current == nil {
		e.t.Fatalf("no pending link: %v", err)
	}
	if _, err := e.tg.ConfirmLink(e.ctx, authctx.FromGrant(g), current.PublicID); err != nil {
		e.t.Fatal(err)
	}
}

func TestTelegramEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("database integration test")
	}
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{URL: url, MaxConns: 8, MinConns: 1,
		MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute, StatementTimeout: 20 * time.Second})
	if err != nil {
		t.Skipf("cannot connect: %v", err)
	}
	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := db.PendingCount(ctx, migrations); err != nil || n > 0 {
		t.Fatalf("pending migrations: %d %v", n, err)
	}

	resolver := rbac.NewResolver(db)
	model := &scriptedModel{}
	repo := assistantPostgres.NewRepository(db)
	registry := tools.NewRegistry(repo, handles.NewSigner("e2e-secret-e2e-secret-e2e-secret-00"), repo, nil)
	registry.SetDatasets(datasets.Default(), repo, repo)
	svc := assistant.NewService(repo, model, registry, nil)
	capsule := newCapsuleBridge("https://dawa24.test")
	capsule.bind(svc, repo, func(int64) bool { return true })

	tg := telegram.NewService(telegramPostgres.New(db), resolver, capsule.forChannel(actions.ChannelTelegram),
		telegram.Config{BotUsername: "Dawa24TestBot", BaseURL: "https://dawa24.test"}, nil)
	router := chi.NewRouter()
	telegramHTTP.NewBridge(tg, e2eToken, nil).RegisterRoutes(router)
	srv := httptest.NewServer(router)
	defer srv.Close()

	e := &e2e{t: t, db: db, ctx: database.AsSystem(ctx), srv: srv, tg: tg, grants: resolver, update: time.Now().UnixNano()}
	stamp := time.Now().UnixNano()

	// A pharmacy with an owner and two employees. The pharmacist's role holds
	// the assistant but not orders; the cashier's role holds neither.
	org := e.scalar(`INSERT INTO org.organizations (name, type, status, legal_name, trade_name)
		VALUES ('{"ar":"صيدلية e2e","en":"E2E"}', 'customer', 'approved', 'E2E', '{"ar":"صيدلية e2e","en":"E2E"}') RETURNING id;`)
	user := func(tag string) int64 {
		return e.scalar(`INSERT INTO identity.users (email, password_hash, name, role, status)
			VALUES ($1, 'x', '{"ar":"مستخدم","en":"User"}', 'customer', 'active') RETURNING id;`,
			fmt.Sprintf("e2e-%s-%d@example.test", tag, stamp))
	}
	owner, pharmacist, cashier := user("owner"), user("pharmacist"), user("cashier")
	role := e.scalar(`INSERT INTO org.roles (organization_id, key, name) VALUES ($1, $2, '{"ar":"صيدلي","en":"Pharmacist"}') RETURNING id;`,
		org, fmt.Sprintf("e2e_pharmacist_%d", stamp))
	e.exec(`INSERT INTO org.role_permissions (role_id, permission_key) VALUES ($1, 'pharmacy.assistant.use');`, role)
	e.exec(`INSERT INTO org.members (organization_id, user_id, role_key, status, is_active) VALUES ($1, $2, 'org_owner', 'active', true);`, org, owner)
	e.exec(`INSERT INTO org.members (organization_id, user_id, role_key, org_role_id, status, is_active) VALUES ($1, $2, 'org_pharmacist', $3, 'active', true);`, org, pharmacist, role)
	e.exec(`INSERT INTO org.members (organization_id, user_id, role_key, status, is_active) VALUES ($1, $2, 'org_employee', 'active', true);`, org, cashier)

	tgOwner, tgPharmacist, tgCashier, tgStranger := stamp%1e9, stamp%1e9+1, stamp%1e9+2, stamp%1e9+3

	t.Run("bridge refuses a caller without the token", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+telegramHTTP.Prefix+"/updates", strings.NewReader(`{"update_id":1}`))
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status %v %v", res.StatusCode, err)
		}
	})

	t.Run("an unlinked Telegram user gets nothing", func(t *testing.T) {
		if got := e.say(tgStranger, "اعرض طلبات الصيدلية"); !strings.Contains(got, "اربط حسابك") {
			t.Fatalf("got %q", got)
		}
	})

	e.link(owner, org, tgOwner)
	e.link(pharmacist, org, tgPharmacist)
	e.link(cashier, org, tgCashier)

	t.Run("owner question runs the real tools under the owner's tenant", func(t *testing.T) {
		got := e.say(tgOwner, "كم طلب عندي؟")
		if !strings.Contains(got, "<b>0</b>") {
			t.Fatalf("answer not delivered: %q", got)
		}
		var decision, agent string
		var auditOrg int64
		if err := db.Pool().QueryRow(ctx, `
			SELECT decision, agent_role, organization_id FROM assistant.tool_audit
			 WHERE user_id = $1 AND tool_name = 'query_data' ORDER BY id DESC LIMIT 1;`, owner,
		).Scan(&decision, &agent, &auditOrg); err != nil {
			t.Fatal(err)
		}
		if decision != "allowed" || agent != "pharmacy" || auditOrg != org {
			t.Fatalf("audit = %s/%s/%d", decision, agent, auditOrg)
		}
		var convOrg int64
		if err := db.Pool().QueryRow(ctx, `
			SELECT c.organization_id FROM telegram.links k JOIN assistant.conversations c ON c.id = k.conversation_id
			 WHERE k.telegram_user_id = $1 AND k.status = 'active';`, tgOwner).Scan(&convOrg); err != nil || convOrg != org {
			t.Fatalf("conversation not persisted for the org: %d %v", convOrg, err)
		}
		model.mu.Lock()
		lastReq := model.calls[len(model.calls)-1]
		model.mu.Unlock()
		if lastReq.UserID != owner || lastReq.OrgID != org {
			t.Fatalf("model called for user %d org %d", lastReq.UserID, lastReq.OrgID)
		}
	})

	t.Run("a role with the assistant but without orders cannot read the orders dataset", func(t *testing.T) {
		e.say(tgPharmacist, "كم طلب عندي؟")
		var decision string
		if err := db.Pool().QueryRow(ctx, `
			SELECT decision FROM assistant.tool_audit
			 WHERE user_id = $1 AND tool_name = 'query_data' ORDER BY id DESC LIMIT 1;`, pharmacist,
		).Scan(&decision); err != nil {
			t.Fatal(err)
		}
		// query_data itself is offered to every assistant user; the dataset
		// it names is what the role is refused, before any SQL runs.
		if decision != "invalid_args" {
			t.Fatalf("decision = %s, want invalid_args", decision)
		}
	})

	t.Run("a role without the assistant never reaches it", func(t *testing.T) {
		got := e.say(tgCashier, "كم طلب عندي؟")
		var turns int
		_ = db.Pool().QueryRow(ctx, `SELECT count(*) FROM assistant.turns WHERE user_id = $1;`, cashier).Scan(&turns)
		if turns != 0 || !strings.Contains(got, "غير مفعّل") {
			t.Fatalf("cashier: %d turns, reply %q", turns, got)
		}
	})

	t.Run("notifications follow the live permission", func(t *testing.T) {
		notify := func(userID int64, perm string) {
			e.exec(`INSERT INTO notifications.logs (user_id, organization_id, channel, recipient, title, body, status, required_permission, sent_at)
				VALUES ($1, $2, 'in_app', 'user', 'تحديث حالة الطلب <#1>', 'تم الشحن', 'sent', $3, now());`, userID, org, perm)
		}
		notify(owner, "pharmacy.order.view")
		notify(pharmacist, "pharmacy.order.view") // the pharmacist's role cannot see orders

		code, out := e.post("/outbox/claim", `{"limit":50}`)
		if code != http.StatusOK {
			t.Fatalf("claim status %d", code)
		}
		chats := map[string]string{}
		var ids []string
		for _, d := range out["deliveries"].([]any) {
			dm := d.(map[string]any)
			chats[dm["chat_id"].(string)] += dm["text"].(string)
			ids = append(ids, fmt.Sprintf(`{"id":%q,"ok":true}`, dm["id"].(string)))
		}
		if !strings.Contains(chats[fmt.Sprint(tgOwner)], "&lt;#1&gt;") {
			t.Fatalf("owner notification missing or unescaped: %v", chats)
		}
		if strings.Contains(chats[fmt.Sprint(tgPharmacist)], "تحديث حالة الطلب") {
			t.Fatal("a notification leaked to a role without the permission")
		}
		if code, _ := e.post("/outbox/report", `{"results":[`+strings.Join(ids, ",")+`]}`); code != http.StatusOK {
			t.Fatalf("report status %d", code)
		}
		var reason string
		_ = db.Pool().QueryRow(ctx, `
			SELECT d.drop_reason FROM telegram.deliveries d JOIN telegram.links k ON k.id = d.link_id
			 WHERE k.user_id = $1 AND d.kind = 'notification';`, pharmacist).Scan(&reason)
		if reason != "permission_revoked" {
			t.Fatalf("pharmacist decision = %q", reason)
		}
	})

	t.Run("suspension and deactivation are enforced on the next message", func(t *testing.T) {
		// Through the real admin write paths, with no manual cache bump: they
		// must invalidate the resolver themselves.
		if err := identityPostgres.NewRepository(db).AdminUpdateUserStatus(ctx, pharmacist, "suspended", owner); err != nil {
			t.Fatal(err)
		}
		member := e.scalar(`SELECT id FROM org.members WHERE organization_id = $1 AND user_id = $2;`, org, owner)
		if err := orgPostgres.NewRepository(db).ToggleMemberStatus(ctx, org, member); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5500 * time.Millisecond) // the resolver re-reads versions every five seconds
		if got := e.say(tgPharmacist, "كم طلب عندي؟"); !strings.Contains(got, "غير نشط") {
			t.Fatalf("suspended user still answered: %q", got)
		}
		if got := e.say(tgOwner, "كم طلب عندي؟"); !strings.Contains(got, "لم تعد لديك عضوية") {
			t.Fatalf("deactivated member still answered: %q", got)
		}
	})
}
