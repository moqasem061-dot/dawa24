package telegram

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/chatbridge"
)

type typingTransport func(*http.Request) (*http.Response, error)

func (f typingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type slowTypingAssistant struct{ fakeAssistant }

func (a *slowTypingAssistant) Ask(context.Context, authctx.Actor, int64, string, ...string) chatbridge.Answer {
	time.Sleep(13 * time.Second)
	return chatbridge.Answer{Markdown: "الإجابة جاهزة"}
}

func TestTypingContinuesDuringLongAnswerAndStopsBeforeReturn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := telegramActionClient
		defer func() { telegramActionClient = original }()
		calls := 0
		telegramActionClient = &http.Client{Transport: typingTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if err := r.ParseForm(); err != nil || r.Form.Get("chat_id") != "5001" || r.Form.Get("action") != "typing" {
				t.Errorf("unexpected typing action: %v, %v", r.Form, err)
			}
			if r.Method != http.MethodPost || r.URL.Host != "api.telegram.org" || !strings.HasSuffix(r.URL.Path, "/sendChatAction") {
				t.Error("unexpected typing endpoint")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
		})}
		repo := newFakeRepo()
		activeLink(repo, userA, tgA, int64p(orgOne))
		grants := fakeGrants{{userA, orgOne}: memberGrant(userA, orgOne, "customer", "approved", "pharmacy.assistant.use")}
		asst := &slowTypingAssistant{fakeAssistant{gate: "pharmacy.assistant.use"}}
		svc := NewService(repo, grants, asst, Config{BotToken: "test-token"}, nil)
		reply := mustHandle(t, svc, privateText(tgA, "ابحث عن دواء"))
		if calls != 4 || !strings.Contains(texts(reply), "الإجابة جاهزة") {
			t.Fatalf("typing calls=%d, reply=%q; want four refreshes during answer", calls, texts(reply))
		}
		time.Sleep(12 * time.Second)
		synctest.Wait()
		if calls != 4 {
			t.Fatal("typing continued after answer returned")
		}
	})
}

func TestTypingStopCancelsInflightAction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := telegramActionClient
		defer func() { telegramActionClient = original }()
		started, canceled := false, false
		telegramActionClient = &http.Client{Transport: typingTransport(func(r *http.Request) (*http.Response, error) {
			started = true
			<-r.Context().Done()
			canceled = true
			return nil, r.Context().Err()
		})}
		svc := NewService(nil, nil, nil, Config{BotToken: "test-token"}, nil)
		stop := svc.keepTyping(context.Background(), tgA)
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if !started {
			t.Fatal("typing did not start")
		}
		stop()
		if !canceled {
			t.Fatal("stop returned before in-flight action was canceled")
		}
	})
}

func TestTypingFailureDoesNotExposeToken(t *testing.T) {
	original := telegramActionClient
	defer func() { telegramActionClient = original }()
	telegramActionClient = &http.Client{Transport: typingTransport(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New(r.URL.String())
	})}
	var logs bytes.Buffer
	svc := NewService(nil, nil, nil, Config{BotToken: "private-test-token"}, slog.New(slog.NewTextHandler(&logs, nil)))
	svc.sendTyping(context.Background(), tgA)
	if strings.Contains(logs.String(), "private-test-token") || strings.Contains(logs.String(), "api.telegram.org") {
		t.Fatal("typing failure logged a credential-bearing URL")
	}
}
