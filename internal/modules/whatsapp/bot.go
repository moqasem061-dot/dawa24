package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// waMarkup formats the shared command texts with WhatsApp's styling marks.
type waMarkup struct{}

func (waMarkup) Escape(s string) string { return s }
func (waMarkup) Bold(s string) string   { return "*" + s + "*" }
func (waMarkup) Code(s string) string   { return "`" + s + "`" }

var (
	reWAID = regexp.MustCompile(`^[0-9]{6,20}$`)
	// reLinkMessage is the text a wa.me link pre-fills: the prefix and a
	// 43-character base64url code. The prefix may be edited away.
	reLinkMessage = regexp.MustCompile(`^(?:ربط\s+Dawa24\s+|/start\s+)?([A-Za-z0-9_-]{43})$`)
)

// HandleWebhook answers the messages in one WhatsApp webhook.
//
// It returns an error only for infrastructure failures. Everything a user can
// cause — an unknown command, an expired code, a missing permission — is an
// ordinary reply, so n8n never has to interpret a failure. Status updates and
// any other change carry no messages and produce no reply.
func (s *Service) HandleWebhook(ctx context.Context, w Webhook) (Reply, error) {
	reply := Reply{Messages: []OutMessage{}}
	for _, m := range w.messages() {
		out, err := s.handleMessage(ctx, m)
		if err != nil {
			return reply, err
		}
		reply.Messages = append(reply.Messages, out...)
	}
	return reply, nil
}

// handleMessage answers one message.
//
// Group messages are ignored entirely. An assistant answer is a pharmacy's or
// supplier's trading position; in a group it would be read by everyone in it.
func (s *Service) handleMessage(ctx context.Context, m namedMessage) ([]OutMessage, error) {
	sys := database.AsSystem(ctx)
	if m.ID == "" || m.GroupID != "" || !reWAID.MatchString(m.From) {
		return nil, nil
	}
	first, err := s.repo.MarkMessageProcessed(sys, m.ID)
	if err != nil || !first {
		return nil, err
	}

	to := m.From
	text := ""
	if m.Text != nil {
		text = strings.TrimSpace(m.Text.Body)
	}
	if code := reLinkMessage.FindStringSubmatch(text); code != nil {
		return s.handleLinkCode(sys, m, code[1])
	}

	link, err := s.repo.LiveLinkByWAID(sys, m.From)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return texts(to, s.notLinkedText()), nil
	}
	if link.Status == LinkPending {
		if link.ConfirmExpiresAt != nil && s.now().After(*link.ConfirmExpiresAt) {
			return texts(to, "⌛ انتهت مهلة تأكيد الربط. أنشئ رابطاً جديداً من صفحة الإعدادات في Dawa24."), nil
		}
		return texts(to, "⏳ طلب الربط بانتظار تأكيدك. افتح صفحة الإعدادات في Dawa24 واضغط «تأكيد الربط»."), nil
	}
	if link.Status == LinkBlocked {
		// A message from the number proves it can be reached again.
		if err := s.repo.SetLinkStatus(sys, link.ID, LinkActive); err != nil {
			return nil, err
		}
		link.Status = LinkActive
	}
	if err := s.repo.TouchLink(sys, link.ID, m.name); err != nil {
		s.log.WarnContext(ctx, "whatsapp: touch link", "error", err)
	}

	if m.Interactive != nil && m.Interactive.ButtonReply != nil {
		return s.handleDecision(ctx, link, m.Interactive.ButtonReply.ID)
	}
	return s.handleText(ctx, link, text)
}

// handleText runs a command or a question.
func (s *Service) handleText(ctx context.Context, link *Link, text string) ([]OutMessage, error) {
	to := link.WAID
	cmd, arg := parseCommand(text)
	switch cmd {
	case "start", "help":
		return texts(to, helpText), nil
	case "whoami", "me":
		return s.runCommand(link, func(c *chatbridge.Chat) (string, error) { return s.core.WhoAmI(ctx, c) })
	case "org":
		return s.runCommand(link, func(c *chatbridge.Chat) (string, error) { return s.core.Org(ctx, c, arg) })
	case "notify":
		return s.runCommand(link, func(c *chatbridge.Chat) (string, error) { return s.core.Notify(ctx, c, arg) })
	case "new":
		if err := s.repo.SetConversation(database.AsSystem(ctx), link.ID, nil); err != nil {
			return nil, err
		}
		return texts(to, "🆕 بدأنا محادثة جديدة. اكتب سؤالك."), nil
	case "unlink":
		if err := s.repo.RevokeLink(database.AsSystem(ctx), link.UserID, link.ID); err != nil {
			return nil, err
		}
		s.log.InfoContext(ctx, "whatsapp link revoked from chat", "user_id", link.UserID)
		return texts(to, "تم إلغاء ربط واتساب بحسابك في Dawa24. لن أجيب على أسئلتك ولن تصلك إشعارات حتى تربط الحساب من جديد."), nil
	case "":
	default:
		return texts(to, "لم أتعرف على هذا الأمر.\n\n"+helpText), nil
	}
	if text == "" {
		return texts(to, "أستقبل حالياً الأسئلة المكتوبة فقط. اكتب سؤالك نصاً."), nil
	}
	return s.answer(ctx, link, text)
}

func (s *Service) runCommand(link *Link, run func(*chatbridge.Chat) (string, error)) ([]OutMessage, error) {
	chat := link.chat()
	text, err := run(chat)
	if err != nil {
		return nil, err
	}
	link.adopt(chat)
	return texts(link.WAID, text), nil
}

// handleLinkCode is the WhatsApp half of linking.
func (s *Service) handleLinkCode(sys context.Context, m namedMessage, code string) ([]OutMessage, error) {
	to := m.From
	userID, orgID, err := s.repo.ConsumeLinkToken(sys, hashCode(code))
	if errors.Is(err, ErrTokenInvalid) {
		return texts(to, "هذا الرمز غير صالح أو مستخدم أو منتهي الصلاحية. أنشئ رابطاً جديداً من صفحة الإعدادات في Dawa24."), nil
	}
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.LiveLinkByWAID(sys, m.From)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.UserID == userID && existing.Status != LinkPending {
		if existing.Status == LinkBlocked {
			if err := s.repo.SetLinkStatus(sys, existing.ID, LinkActive); err != nil {
				return nil, err
			}
		}
		return texts(to, "رقم واتساب هذا مربوط بحسابك في Dawa24 بالفعل. اكتب سؤالك أو /help."), nil
	}

	link := &Link{UserID: userID, WAID: m.From, DisplayName: m.name, ActiveOrgID: orgID}
	err = s.repo.CreatePendingLink(sys, link, s.now().Add(confirmTTL))
	if errors.Is(err, ErrLinkedElsewhere) {
		return texts(to, "رقم واتساب هذا مربوط بحساب آخر في Dawa24. أرسل /unlink من هنا أولاً ثم أعد المحاولة."), nil
	}
	if err != nil {
		return nil, err
	}
	// Deliberately no account name or email here: whoever sent the code may
	// not be its owner, and learning whose account it was is already a leak.
	return texts(to,
		"🔗 *تم استلام طلب الربط.*\n\n"+
			"لإكماله ارجع إلى صفحة الإعدادات في Dawa24 التي أنشأت منها الرابط، وتحقق من أن رقم واتساب الظاهر هو رقمك، ثم اضغط «تأكيد الربط».\n\n"+
			"تنتهي المهلة خلال 10 دقائق. إذا لم تطلب هذا الربط فتجاهل الرسالة."), nil
}

// answer runs one question through the shared gates and the assistant.
func (s *Service) answer(ctx context.Context, link *Link, question string) ([]OutMessage, error) {
	chat := link.chat()
	ans, refusal, err := s.core.Ask(ctx, chat, question)
	if err != nil {
		return nil, err
	}
	link.adopt(chat)
	if refusal != "" {
		return texts(link.WAID, refusal), nil
	}
	out := texts(link.WAID, formatAnswer(ans)...)
	for _, p := range ans.Proposals {
		out = append(out, proposalMessage(link.WAID, p))
	}
	for _, f := range ans.Files {
		if m, ok := s.documentMessage(link.WAID, f); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// formatAnswer turns an assistant answer into WhatsApp text messages.
func formatAnswer(ans chatbridge.Answer) []string {
	chunks := RenderMarkdown(ans.Markdown)
	if ans.Failure != "" {
		chunks = append(chunks, "⚠️ "+ans.Failure)
	}
	if links := renderLinks(ans.Links); links != "" {
		if n := len(chunks); n > 0 && units(chunks[n-1])+units(links)+2 <= MaxMessageUnits {
			chunks[n-1] += "\n\n" + links
		} else {
			chunks = append(chunks, links)
		}
	}
	if len(chunks) == 0 {
		chunks = []string{"لم أتمكن من صياغة إجابة لهذا السؤال. جرّب صياغة أوضح."}
	}
	return chunks
}

const maxAnswerLinks = 6

func renderLinks(links []chatbridge.AnswerLink) string {
	var b strings.Builder
	n := 0
	for _, l := range links {
		u := safeHTTPURL(l.URL)
		if u == "" || strings.TrimSpace(l.Title) == "" {
			continue
		}
		if n == 0 {
			b.WriteString("🔗 *فتح في Dawa24*")
		}
		fmt.Fprintf(&b, "\n• %s: %s", strings.TrimSpace(l.Title), u)
		n++
		if n == maxAnswerLinks {
			break
		}
	}
	return b.String()
}

// parseCommand splits "/cmd arg" into ("cmd", "arg").
func parseCommand(text string) (string, string) {
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	name, arg, _ := strings.Cut(text[1:], " ")
	return strings.ToLower(name), strings.TrimSpace(arg)
}

func texts(to string, bodies ...string) []OutMessage {
	out := make([]OutMessage, 0, len(bodies))
	for _, b := range bodies {
		if b != "" {
			out = append(out, OutMessage{To: to, Payload: textPayload(to, b)})
		}
	}
	return out
}

func (s *Service) notLinkedText() string {
	settings := "صفحة الإعدادات"
	if s.cfg.BaseURL != "" {
		settings += " (" + s.cfg.BaseURL + "/settings#whatsapp)"
	}
	return "👋 أهلاً بك في *Dawa24* على واتساب.\n\n" +
		"لاستخدام المساعد كبسولة واستقبال إشعارات منشأتك، اربط رقمك أولاً:\n" +
		"1. سجّل الدخول إلى Dawa24 وافتح " + settings + ".\n" +
		"2. اضغط «ربط واتساب» وأرسل الرسالة التي تظهر لك.\n" +
		"3. أكّد الربط من صفحة الإعدادات.\n\n" +
		"لن نطلب منك كلمة المرور هنا أبداً."
}

const helpText = "🤖 *مساعد Dawa24 كبسولة على واتساب*\n\n" +
	"اكتب سؤالك مباشرة عن طلباتك أو مبيعاتك أو محفظتك أو منتجاتك، وسأجيب من بيانات منشأتك حسب صلاحياتك.\n\n" +
	"*الأوامر:*\n" +
	"/whoami — الحساب والمنشأة والفرع والصلاحيات الحالية\n" +
	"/org — عرض منشآتك وتبديل المنشأة النشطة\n" +
	"/new — بدء محادثة جديدة\n" +
	"/notify — التحكم في إشعارات واتساب\n" +
	"/unlink — إلغاء ربط واتساب بحسابك\n" +
	"/help — عرض هذه الرسالة"
