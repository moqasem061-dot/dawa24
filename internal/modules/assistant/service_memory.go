package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// memoryTriggerKeywords is the fast heuristic filter run on every turn.
//
// 99% of queries are transient ("كم أنفقت", "أين طلبيتي", etc.). We only invoke
// the LLM extraction capability when the user's text contains keywords that
// indicate enduring organizational policies, operational rules, or preferences.
var memoryTriggerKeywords = []string{
	"نحن", "لدينا", "صيدليتنا", "شركتنا", "مؤسستنا", "فرعنا", "فروعنا", "مخزننا", "مستودعنا",
	"نفضل", "تفضيلنا", "دائماً", "عادة", "ممنوع", "لا نريد", "نرفض", "لا نقبل",
	"مواعيدنا", "دوامنا", "ساعات عملنا", "عنواننا", "موقعنا", "عنوان الفرع",
	"شروطنا", "طريقة دفعنا", "سدادنا", "ميزانيتنا", "حدنا الائتماني", "شيك", "كاش", "أجل",
	"تذكر", "احفظ عندك", "لا تنسى", "اعلم أن", "سجل عندك", "خل في بالك", "قاعدتنا",
}

func hasMemoryTrigger(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range memoryTriggerKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

const memoryExtractionSystemPrompt = `أنت خبير استخراج معلومات الأعمال للمساعد الذكي "دكتور كبسولة" في منصة دوا 24.
مهمتك: تحليل رسالة المستخدم والرد عليها لتحديد ما إذا صرّح المستخدم بمعلومة أو حقيقة أو قاعدة أو قيد أو تفضيل دائم ومحدد يخص منشأته (صيدلية أو مورّد).

القواعد الصارمة:
1. استخرج فقط الحقائق الدائمة الثابتة للمنشأة (مثل: مواعيد الاستلام وساعات العمل، سياسات الدفع وشروطه، اشتراطات الفروع، تفضيلات الموردين أو الماركات).
2. لا تستخرج الأسئلة اللحظية أو طلبات التحليل العابرة أو أرقام الفواتير المؤقتة.
3. صغ كل حقيقة بأسلوب جملة خبرية واضحة ومباشرة وموجزة بالعربية (مثال: "مواعيد استلام طلبيات فرع المعادي من 10 صباحاً إلى 3 عصراً فقط").
4. التصنيفات المسموحة فقط:
   - business_profile: معلومات عامة عن طبيعة نشاط المنشأة وفروعها.
   - procurement: تفضيلات وسياسات الشراء والمخزون.
   - financial: سياسات وشروط السداد، الكاش، الشيكات، الائتمان.
   - logistics: مواعيد الاستلام والتوصيل والشحن والتخزين.
   - preferences: تفضيلات التعامل والتحليل والتقارير.
   - contacts: أرقام أو جهات الاتصال والمسؤولين.
   - general: أي قاعدة تشغيلية دائمة أخرى.
5. الإخراج يجب أن يكون بتنسيق JSON حصراً:
   {"memories": [{"content": "...", "category": "..."}]}
   إذا لم تجد أي حقيقة دائمة تخص المنشأة، أعد مصفوفة فارغة: {"memories": []}`

type memoryExtractResponse struct {
	Memories []struct {
		Content  string `json:"content"`
		Category string `json:"category"`
	} `json:"memories"`
}

// maybeExtractMemory runs asynchronously after a turn succeeds, detecting and
// saving durable organization facts without blocking user interaction.
func (s *Service) maybeExtractMemory(parentCtx context.Context, actor authctx.Actor, question, answer string) {
	if actor.OrgID <= 0 || s.gateway == nil || !s.gateway.Enabled() || s.repo == nil {
		return
	}
	qTrim := strings.TrimSpace(question)
	if len(qTrim) < 12 {
		return
	}
	if !hasMemoryTrigger(qTrim) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	var virtualKey string
	if s.keys != nil {
		if vk, err := s.keys(ctx, actor.OrgID); err == nil {
			virtualKey = vk
		}
	}

	userInput := fmt.Sprintf("رسالة المستخدم: %s\n\nرد المساعد: %s", qTrim, answer)
	req := gateway.Request{
		Capability:     gateway.CapMemoryExtract,
		System:         memoryExtractionSystemPrompt,
		Input:          userInput,
		OrganizationID: actor.OrgID,
		UserID:         actor.UserID,
		Feature:        "ذاكرة دكتور كبسولة",
		VirtualKey:     virtualKey,
		MaxTokens:      500,
	}

	resp, err := s.gateway.Invoke(ctx, req)
	if err != nil {
		s.log.DebugContext(ctx, "assistant: memory extraction skipped or failed", "org_id", actor.OrgID, "error", err)
		return
	}

	raw := strings.TrimSpace(resp.Content)
	// Strip markdown code fences if model returned ```json ... ```
	if strings.HasPrefix(raw, "```") {
		idx := strings.Index(raw, "\n")
		if idx > 0 {
			raw = raw[idx+1:]
		}
		if last := strings.LastIndex(raw, "```"); last >= 0 {
			raw = raw[:last]
		}
		raw = strings.TrimSpace(raw)
	}

	var parsed memoryExtractResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		s.log.DebugContext(ctx, "assistant: memory extract json parse error", "org_id", actor.OrgID, "raw", raw, "error", err)
		return
	}

	for _, item := range parsed.Memories {
		content := strings.TrimSpace(item.Content)
		if len(content) < 8 {
			continue
		}
		if len(content) > 500 {
			content = content[:500]
		}

		cat := MemoryCategory(strings.TrimSpace(item.Category))
		switch cat {
		case MemoryCategoryProfile, MemoryCategoryProcurement, MemoryCategoryFinancial,
			MemoryCategoryLogistics, MemoryCategoryPreferences, MemoryCategoryContacts:
		default:
			cat = MemoryCategoryGeneral
		}

		// Deduplication check: see if a very similar memory already exists
		existing, err := s.repo.FindMemories(ctx, actor.OrgID, content, 3)
		if err == nil && len(existing) > 0 {
			isDupe := false
			for _, ex := range existing {
				if strings.TrimSpace(ex.Content) == content || strings.Contains(ex.Content, content) || strings.Contains(content, ex.Content) {
					isDupe = true
					break
				}
			}
			if isDupe {
				continue
			}
		}

		mem := &Memory{
			OrganizationID: actor.OrgID,
			Scope:          MemoryScopeOrganization,
			Category:       cat,
			Content:        content,
			Source:         MemorySourceAIExtracted,
			IsActive:       true,
		}

		err = s.repo.SaveMemory(ctx, mem)
		if err != nil {
			s.log.WarnContext(ctx, "assistant: failed to auto-save extracted memory", "org_id", actor.OrgID, "error", err)
			continue
		}

		s.log.InfoContext(ctx, "assistant: memory learned autonomously",
			"org_id", actor.OrgID, "memory_id", mem.ID, "category", mem.Category, "content", mem.Content)
	}
}
