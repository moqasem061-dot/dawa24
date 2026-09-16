package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/chatbridge"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// HandleUpdate answers one Telegram update.
//
// It returns an error only for infrastructure failures. Everything a user can
// cause — an unknown command, an expired code, a missing permission — is an
// ordinary reply, so n8n never has to interpret a failure.
//
// Groups and channels are ignored entirely. An assistant answer is a pharmacy's
// or supplier's trading position; in a group it would be read by everyone in
// the chat, whoever they are.
func (s *Service) HandleUpdate(ctx context.Context, u Update) (Reply, error) {
	sys := database.AsSystem(ctx)

	if m := u.MyChatMember; m != nil {
		if m.Chat.Type == "private" {
			if first, err := s.repo.MarkUpdateProcessed(sys, u.UpdateID); err != nil || !first {
				return Reply{}, err
			}
			return Reply{}, s.onMembershipChange(sys, m)
		}
		return Reply{}, nil
	}

	if q := u.CallbackQuery; q != nil {
		return s.handleCallback(ctx, u.UpdateID, q)
	}

	msg := u.Message
	if msg == nil || msg.From == nil || msg.From.IsBot || msg.Chat.Type != "private" || msg.Chat.ID != msg.From.ID {
		return Reply{}, nil
	}
	first, err := s.repo.MarkUpdateProcessed(sys, u.UpdateID)
	if err != nil || !first {
		return Reply{}, err
	}

	chatID := msg.Chat.ID
	cmd, arg := parseCommand(msg.Text, s.cfg.BotUsername)
	if cmd == "start" && arg != "" {
		return s.handleLinkCode(sys, msg, arg)
	}

	link, err := s.repo.LiveLinkByTelegramUser(sys, msg.From.ID)
	if err != nil {
		return Reply{}, err
	}
	if link == nil {
		return replyTo(chatID, s.notLinkedText()), nil
	}
	if link.Status == LinkPending {
		if link.ConfirmExpiresAt != nil && s.now().After(*link.ConfirmExpiresAt) {
			return replyTo(chatID, "⌛ انتهت مهلة تأكيد الربط. أنشئ رابطاً جديداً من صفحة الإعدادات في Dawa24."), nil
		}
		return replyTo(chatID, "⏳ طلب الربط بانتظار تأكيدك. افتح صفحة الإعدادات في Dawa24 واضغط «تأكيد الربط»."), nil
	}
	if link.Status == LinkBlocked {
		// Writing to the bot is the clearest possible sign it is unblocked.
		if err := s.repo.SetLinkStatus(sys, link.ID, LinkActive); err != nil {
			return Reply{}, err
		}
		link.Status = LinkActive
	}
	if err := s.repo.TouchLink(sys, link.ID, chatID, msg.From.Username, displayName(msg.From)); err != nil {
		s.log.WarnContext(ctx, "telegram: touch link", "error", err)
	}

	switch cmd {
	case "start", "help":
		return replyTo(chatID, helpText), nil
	case "whoami", "me":
		return s.cmdWhoAmI(ctx, link)
	case "org":
		return s.cmdOrg(ctx, link, arg)
	case "notify":
		return s.cmdNotify(ctx, link, arg)
	case "new":
		if err := s.repo.SetConversation(sys, link.ID, nil); err != nil {
			return Reply{}, err
		}
		return replyTo(chatID, "🆕 بدأنا محادثة جديدة. اكتب سؤالك."), nil
	case "offers", "promotions":
		q := "ما هي العروض والخصومات الخاصة المتاحة لفرعي حالياً؟"
		if arg != "" {
			q = "هل توجد عروض أو خصومات على " + arg + "؟"
		}
		return s.answer(ctx, link, q)
	case "cart":
		q := "ما هي محتويات سلة الشراء الحالية لفرعي وإجمالي قيمتها؟"
		if arg != "" {
			q = "أضف " + arg + " إلى سلة الشراء"
		}
		return s.answer(ctx, link, q)
	case "orders":
		q := "ما هي آخر طلبات الشراء وحالتها الحالية؟"
		if arg != "" {
			q = "ما هي تفاصيل وحالة الطلب رقم " + arg + "؟"
		}
		return s.answer(ctx, link, q)
	case "reorder":
		q := "ما هي مقترحات إعادة طلب الأدوية والنواقص لفرعي؟"
		if arg != "" {
			q = "هل يوجد اقتراح لإعادة طلب " + arg + "؟"
		}
		return s.answer(ctx, link, q)
	case "wallet":
		return s.answer(ctx, link, "ما هو رصيد المحفظة الحالي وآخر الحركات المالية؟")
	case "unlink":
		if err := s.repo.RevokeLink(sys, link.UserID, link.ID); err != nil {
			return Reply{}, err
		}
		s.log.InfoContext(ctx, "telegram link revoked from chat", "user_id", link.UserID)
		return replyTo(chatID, "تم إلغاء ربط تيليجرام بحسابك في Dawa24. لن يجيب البوت على أسئلتك ولن تصلك إشعارات حتى تربط الحساب من جديد."), nil
	case "":
	default:
		return replyTo(chatID, "لم أتعرف على هذا الأمر.\n\n"+helpText), nil
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}

	var attachmentRefs []string
	if msg.Document != nil || len(msg.Photo) > 0 {
		ref, err := s.ingestTelegramAttachment(ctx, link, msg)
		if err != nil {
			s.log.WarnContext(ctx, "telegram: ingest attachment failed", "error", err)
			return replyTo(chatID, "تعذّر استلام أو قراءة الملف المرفق: "+err.Error()), nil
		}
		if ref != "" {
			attachmentRefs = append(attachmentRefs, ref)
		}
	}

	if text == "" && len(attachmentRefs) == 0 {
		return replyTo(chatID, "أستقبل حالياً الأسئلة المكتوبة أو المرفقات (ملفات Excel أو صور الروشتات). اكتب سؤالك أو أرفق الملف."), nil
	}
	return s.answer(ctx, link, text, attachmentRefs...)
}

// handleLinkCode is the Telegram half of linking.
func (s *Service) handleLinkCode(sys context.Context, msg *Message, code string) (Reply, error) {
	chatID := msg.Chat.ID
	invalid := replyTo(chatID, "هذا الرابط غير صالح أو مستخدم أو منتهي الصلاحية. أنشئ رابطاً جديداً من صفحة الإعدادات في Dawa24.")
	if len(code) > 64 {
		return invalid, nil
	}
	userID, orgID, err := s.repo.ConsumeLinkToken(sys, hashCode(code))
	if errors.Is(err, ErrTokenInvalid) {
		return invalid, nil
	}
	if err != nil {
		return Reply{}, err
	}

	existing, err := s.repo.LiveLinkByTelegramUser(sys, msg.From.ID)
	if err != nil {
		return Reply{}, err
	}
	if existing != nil && existing.UserID == userID && existing.Status != LinkPending {
		if existing.Status == LinkBlocked {
			if err := s.repo.SetLinkStatus(sys, existing.ID, LinkActive); err != nil {
				return Reply{}, err
			}
		}
		return replyTo(chatID, "حساب تيليجرام هذا مربوط بحسابك في Dawa24 بالفعل. اكتب سؤالك أو /help."), nil
	}

	link := &Link{
		UserID:         userID,
		TelegramUserID: msg.From.ID,
		ChatID:         chatID,
		Username:       msg.From.Username,
		DisplayName:    displayName(msg.From),
		ActiveOrgID:    orgID,
	}
	err = s.repo.CreatePendingLink(sys, link, s.now().Add(confirmTTL))
	if errors.Is(err, ErrLinkedElsewhere) {
		return replyTo(chatID, "حساب تيليجرام هذا مربوط بحساب آخر في Dawa24. أرسل /unlink من هنا أولاً ثم أعد المحاولة."), nil
	}
	if err != nil {
		return Reply{}, err
	}
	// Deliberately no account name or email here: whoever opened the code may
	// not be its owner, and learning whose account it was is already a leak.
	return replyTo(chatID,
		"🔗 <b>تم استلام طلب الربط.</b>\n\n"+
			"لإكماله ارجع إلى صفحة الإعدادات في Dawa24 التي أنشأت منها الرابط، وتحقق من أن اسم حساب تيليجرام الظاهر هو حسابك، ثم اضغط «تأكيد الربط».\n\n"+
			"تنتهي المهلة خلال 10 دقائق. إذا لم تطلب هذا الربط فتجاهل الرسالة."), nil
}

// onMembershipChange tracks the user blocking and unblocking the bot.
func (s *Service) onMembershipChange(sys context.Context, m *ChatMemberUpdated) error {
	link, err := s.repo.LiveLinkByTelegramUser(sys, m.From.ID)
	if err != nil || link == nil {
		return err
	}
	switch m.NewChatMember.Status {
	case "kicked":
		if link.Status == LinkActive {
			return s.repo.SetLinkStatus(sys, link.ID, LinkBlocked)
		}
	case "member":
		if link.Status == LinkBlocked {
			return s.repo.SetLinkStatus(sys, link.ID, LinkActive)
		}
	}
	return nil
}

func (s *Service) ingestTelegramAttachment(ctx context.Context, link *Link, msg *Message) (string, error) {
	actor, refusal, err := s.core.Authorize(ctx, link.chat())
	if err != nil {
		return "", err
	}
	if refusal != "" {
		return "", errors.New(refusal)
	}

	var (
		content  []byte
		filename string
	)

	if msg.Document != nil {
		doc := msg.Document
		filename = doc.FileName
		if filename == "" {
			filename = "telegram_document"
		}
		content, err = s.DownloadFile(ctx, doc.FileID, doc.FileURL, doc.FileData)
		if err != nil {
			return "", err
		}
	} else if len(msg.Photo) > 0 {
		// Pick highest resolution photo
		p := msg.Photo[len(msg.Photo)-1]
		filename = fmt.Sprintf("telegram_photo_%d.jpg", time.Now().Unix())
		content, err = s.DownloadFile(ctx, p.FileID, p.FileURL, p.FileData)
		if err != nil {
			return "", err
		}
	} else {
		return "", nil
	}

	if len(content) == 0 {
		return "", errors.New("الملف المرفق فارغ")
	}

	return s.assistant.IngestAttachment(ctx, actor, filename, content)
}

// answer runs one question through the shared gates and the assistant.
func (s *Service) answer(ctx context.Context, link *Link, question string, attachmentRefs ...string) (Reply, error) {
	chat := link.chat()
	ans, refusal, err := s.core.Ask(ctx, chat, question, attachmentRefs...)
	if err != nil {
		return Reply{}, err
	}
	link.adopt(chat)
	if refusal != "" {
		return replyTo(link.ChatID, refusal), nil
	}
	reply := replyTo(link.ChatID, formatAnswer(ans)...)
	for _, p := range ans.Proposals {
		reply.Messages = append(reply.Messages, proposalMessage(link.ChatID, p))
	}
	for _, f := range ans.Files {
		if m, ok := s.documentMessage(link.ChatID, f); ok {
			reply.Messages = append(reply.Messages, m)
		}
	}
	return reply, nil
}

// formatAnswer turns an assistant answer into Telegram messages.
func formatAnswer(ans chatbridge.Answer) []string {
	chunks := RenderMarkdown(ans.Markdown)
	if ans.Failure != "" {
		chunks = append(chunks, "⚠️ "+escape(ans.Failure))
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
			b.WriteString("🔗 <b>فتح في Dawa24</b>")
		}
		b.WriteString("\n• <a href=\"" + escape(u) + "\">" + escape(l.Title) + "</a>")
		n++
		if n == maxAnswerLinks {
			break
		}
	}
	return b.String()
}

// parseCommand splits "/cmd@bot arg" into ("cmd", "arg"). A command addressed
// to a different bot is not a command.
func parseCommand(text, bot string) (string, string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	head, arg, _ := strings.Cut(text[1:], " ")
	name, target, hasTarget := strings.Cut(head, "@")
	if hasTarget && bot != "" && !strings.EqualFold(target, bot) {
		return "", ""
	}
	return strings.ToLower(name), strings.TrimSpace(arg)
}

func displayName(u *User) string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

func (s *Service) notLinkedText() string {
	settings := "صفحة الإعدادات"
	if s.cfg.BaseURL != "" {
		settings = `<a href="` + escape(s.cfg.BaseURL+"/settings#telegram") + `">صفحة الإعدادات</a>`
	}
	return "👋 أهلاً بك في بوت <b>Dawa24</b>.\n\n" +
		"لاستخدام المساعد كبسولة واستقبال إشعارات منشأتك، اربط حسابك أولاً:\n" +
		"1. سجّل الدخول إلى Dawa24 وافتح " + settings + ".\n" +
		"2. اضغط «ربط تيليجرام» وافتح الرابط الذي يظهر لك.\n" +
		"3. أكّد الربط من صفحة الإعدادات.\n\n" +
		"لن نطلب منك كلمة المرور هنا أبداً."
}

const helpText = "🤖 <b>مساعد Dawa24 كبسولة على تيليجرام</b>\n\n" +
	"اكتب سؤالك مباشرة أو استخدم الأوامر التالية للشراء والاستعلام وإدارة حسابك:\n\n" +
	"<b>أوامر الشراء والعروض:</b>\n" +
	"/offers — استعراض العروض والخصومات الخاصة المتاحة لفرعك\n" +
	"/cart — محتويات سلة الشراء وتجهيز أمر الشراء\n" +
	"/orders — متابعة أحدث طلبات الشراء وحالاتها\n" +
	"/reorder — مقترحات إعادة طلب النواقص والأصناف\n" +
	"/wallet — رصيد المحفظة والحركات المالية الأخيرة\n\n" +
	"<b>إدارة الحساب والمحادثة:</b>\n" +
	"/org — عرض منشآتك وتبديل المنشأة النشطة\n" +
	"/whoami — بيانات الحساب والمنشأة والفرع والصلاحيات\n" +
	"/new — بدء محادثة جديدة وتصفير سياق الأسئلة\n" +
	"/notify — التحكم في إشعارات تيليجرام وتخصيصها\n" +
	"/unlink — إلغاء ربط تيليجرام بحسابك في Dawa24\n" +
	"/help — عرض هذه الرسالة وقائمة الأوامر\n\n" +
	"<b>المرفقات وقوائم الأدوية:</b>\n" +
	"📎 يمكنك إرسال ملفات Excel أو CSV تحتوي على قوائم أصناف، أو صور الروشتات والنواقص مباشرة، وسيقوم المساعد بقراءتها وفحص العروض المتاحة وتجهيز الطلبية لك!"
