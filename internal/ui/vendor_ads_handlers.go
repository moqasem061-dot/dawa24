package ui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// loadVendorInStockItems loads all product variants owned by the vendor that currently have stock > 0.
func (h *UIHandler) loadVendorInStockItems(ctx context.Context, orgID int64) []pages.VendorOfferItemOption {
	var itemOptions []pages.VendorOfferItemOption
	if orgID <= 0 {
		return itemOptions
	}
	var warehouses []*inventory.Warehouse
	var stocks []*inventory.Stock
	whNameMap := make(map[int64]string)
	stockQtyMap := make(map[int64]int)
	stockWhMap := make(map[int64]string)

	if h.invSvc != nil {
		allWhs, _ := h.invSvc.ListWarehouses(ctx)
		stocks, _ = h.invSvc.ListStocksByOrg(ctx, orgID)
		for _, wh := range allWhs {
			if wh != nil && wh.OrganizationID == orgID {
				warehouses = append(warehouses, wh)
				whNameMap[wh.ID] = wh.Name
			}
		}
		for _, s := range stocks {
			if s != nil {
				stockQtyMap[s.ProductVariantID] += s.Quantity
				if n, exists := whNameMap[s.WarehouseID]; exists && stockWhMap[s.ProductVariantID] == "" {
					stockWhMap[s.ProductVariantID] = n
				}
			}
		}
	}

	if h.catSvc != nil {
		variants, _, err := h.catSvc.ListVariantsByOrganization(ctx, orgID, catalog.VariantSearchParams{Limit: 500})
		if err == nil {
			for _, v := range variants {
				if v == nil {
					continue
				}
				whName := stockWhMap[v.ID]
				stockQty := v.StockQty
				if qty, ok := stockQtyMap[v.ID]; ok {
					stockQty = qty
				}
				if stockQty <= 0 {
					continue
				}
				expStr := ""
				if v.ExpiryDate != nil {
					expStr = v.ExpiryDate.Format("2006-01-02")
				}
				itemOptions = append(itemOptions, pages.VendorOfferItemOption{
					VariantID:      v.ID,
					NameAr:         v.Name["ar"],
					NameEn:         v.Name["en"],
					SKU:            v.SKU,
					BatchNumber:    v.BatchNumber,
					ExpiryDate:     expStr,
					Price:          v.Price.String(),
					PriceFloat:     float64(v.Price.Minor()) / 100.0,
					WarehouseName:  whName,
					AvailableStock: stockQty,
				})
			}
		}
	}
	return itemOptions
}

// VendorAdCreateSubmit handles the creation of a new advertisement with direct media upload and 2 credits deduction.
func (h *UIHandler) VendorAdCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	// Max 20MB for direct image/video uploads
	_ = r.ParseMultipartForm(uploadMemoryBudget)

	ad := h.parseAdForm(r, actor.OrganizationID)

	// Everything the wizard checks in the browser, checked again here.
	//
	// The wizard disables its own "next" and "confirm" buttons, and until now
	// that was the only check there was: a POST could name a title-less
	// advertisement against another supplier's variant, for a duration the
	// pricing does not know, and it would be written. A disabled button stops a
	// person, not a request.
	if msg := h.validateAdSubmission(ctx, actor.OrganizationID, ad, lang); msg != "" {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", msg)
		return
	}

	// Check if a direct file was uploaded
	if file, header, fileErr := r.FormFile("media_file"); fileErr == nil && file != nil {
		defer file.Close()
		if data, readErr := io.ReadAll(file); readErr == nil && len(data) > 0 {
			if u, saveErr := saveUploadedBytes(data, header.Filename, "ads"); saveErr == nil && u != "" {
				ad.MediaURL = u
				if strings.HasSuffix(strings.ToLower(header.Filename), ".mp4") || strings.HasSuffix(strings.ToLower(header.Filename), ".webm") {
					ad.MediaType = promo.MediaVideo
				} else {
					ad.MediaType = promo.MediaImage
				}
			}
		}
	}

	created, err := h.promoSvc.CreateAd(ctx, ad)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", h.safeMessage(err, lang))
		return
	}
	_ = created
	h.redirectWithNotice(w, r, "/vendor/ads", "success", i18n.T(lang, "vendor.ads.created_review_success"))
}

// VendorAdUpdateSubmit handles the update of an existing advertisement.
// If the ad is already approved and live, changes are submitted as a pending edit request
// so the live ad continues running smoothly without interruption until admin approval.
func (h *UIHandler) VendorAdUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", i18n.T(lang, "vendor.ads.invalid_ad_id"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	existingAd, err := h.promoSvc.GetAd(sysCtx, id)
	if err != nil || existingAd == nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", "لم يتم العثور على الإعلان المطلوب تعديله")
		return
	}
	if existingAd.OrganizationID != nil && *existingAd.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", "غير مصرح لك بتعديل هذا الإعلان")
		return
	}

	_ = r.ParseMultipartForm(uploadMemoryBudget)
	ad := h.parseAdForm(r, actor.OrganizationID)
	ad.ID = id

	if file, header, fileErr := r.FormFile("media_file"); fileErr == nil && file != nil {
		defer file.Close()
		if data, readErr := io.ReadAll(file); readErr == nil && len(data) > 0 {
			if u, saveErr := saveUploadedBytes(data, header.Filename, "ads"); saveErr == nil && u != "" {
				ad.MediaURL = u
				if strings.HasSuffix(strings.ToLower(header.Filename), ".mp4") || strings.HasSuffix(strings.ToLower(header.Filename), ".webm") {
					ad.MediaType = promo.MediaVideo
				} else {
					ad.MediaType = promo.MediaImage
				}
			}
		}
	} else if ad.MediaURL == "" {
		ad.MediaURL = existingAd.MediaURL
		ad.MediaType = existingAd.MediaType
	}

	ad.StartsAt = existingAd.StartsAt
	ad.ExpiresAt = existingAd.ExpiresAt
	ad.IsActive = existingAd.IsActive

	// If ad is already live and approved, submit an edit request without stopping the live ad
	if existingAd.AdminStatus == promo.AdminApproved {
		changes := &promo.AdPendingChanges{
			TitleAr:         ad.TitleAr,
			TitleEn:         ad.TitleEn,
			AdTextAr:        ad.AdTextAr,
			AdTextEn:        ad.AdTextEn,
			MediaType:       ad.MediaType,
			MediaURL:        ad.MediaURL,
			ThumbnailURL:    ad.ThumbnailURL,
			Position:        ad.Position,
			TargetURL:       ad.TargetURL,
			ClickTargetType: ad.ClickTargetType,
			ClickTargetID:   ad.ClickTargetID,
		}
		if err := h.promoSvc.SubmitAdEditRequest(ctx, id, changes); err != nil {
			h.redirectWithNotice(w, r, "/vendor/ads", "error", h.safeMessage(err, lang))
			return
		}
		h.redirectWithNotice(w, r, "/vendor/ads", "success", i18n.T(lang, "vendor.ads.edit_request_success"))
		return
	}

	// Otherwise (if still draft or pending), update directly
	if err := h.promoSvc.UpdateAd(ctx, ad); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/ads", "success", i18n.T(lang, "vendor.ads.updated_success"))
}

func (h *UIHandler) parseAdForm(r *http.Request, orgID int64) *promo.Ad {
	titleAr := strings.TrimSpace(r.FormValue("title_ar"))
	titleEn := strings.TrimSpace(r.FormValue("title_en"))
	adTextAr := strings.TrimSpace(r.FormValue("ad_text_ar"))
	adTextEn := strings.TrimSpace(r.FormValue("ad_text_en"))
	mediaURL := strings.TrimSpace(r.FormValue("media_url"))
	thumbnailURL := strings.TrimSpace(r.FormValue("thumbnail_url"))
	mediaType := strings.TrimSpace(r.FormValue("media_type"))
	// The destination is folded onto the three the platform accepts, and the
	// placement onto the five it renders. Both used to be written through as
	// submitted, so a crafted POST could store a click target no page knows how
	// to resolve and a placement no slot displays.
	clickTarget := promo.ValidClickTarget(strings.TrimSpace(r.FormValue("click_target_type")))
	targetID := strings.TrimSpace(r.FormValue("click_target_id"))
	position := validAdPosition(strings.TrimSpace(r.FormValue("position")))
	if mediaType == "" {
		mediaType = "image"
	}
	lowerURL := strings.ToLower(mediaURL)
	if strings.HasSuffix(lowerURL, ".mp4") || strings.HasSuffix(lowerURL, ".webm") || strings.HasSuffix(lowerURL, ".mov") {
		mediaType = "video"
	}
	if clickTarget == "" {
		clickTarget = "product"
	}

	var clickTargetID *int64
	if targetID != "" {
		if id, err := strconv.ParseInt(targetID, 10, 64); err == nil && id > 0 {
			clickTargetID = &id
		}
	}

	// Only the four durations the wizard offers, because each is priced.
	durationDays := 30
	if d, err := strconv.Atoi(r.FormValue("duration_days")); err == nil {
		switch d {
		case 7, 15, 30, 60:
			durationDays = d
		}
	}

	title := titleAr
	if title == "" {
		title = titleEn
	}

	// The destination is derived, never submitted.
	//
	// target_url used to be read straight from the form and, because
	// ResolveClickURL prefers it over the click target, a posted URL won
	// outright — which is how an "external link" survived removing the option
	// that offered it. Ads point inside the platform, so the platform builds
	// the path.
	var targetURL string
	switch clickTarget {
	case promo.ClickTargetVendor:
		targetURL = fmt.Sprintf("/suppliers/%d", orgID)
	case promo.ClickTargetOffer:
		if clickTargetID != nil {
			targetURL = fmt.Sprintf("/offers/%d", *clickTargetID)
		} else {
			targetURL = fmt.Sprintf("/suppliers/%d#offers", orgID)
		}
	default:
		if clickTargetID != nil {
			targetURL = fmt.Sprintf("/catalog/%d", *clickTargetID)
		} else {
			targetURL = fmt.Sprintf("/suppliers/%d", orgID)
		}
	}

	return &promo.Ad{
		OrganizationID:  &orgID,
		Title:           title,
		TitleAr:         titleAr,
		TitleEn:         titleEn,
		AdTextAr:        adTextAr,
		AdTextEn:        adTextEn,
		MediaType:       promo.AdMediaType(mediaType),
		MediaURL:        mediaURL,
		ThumbnailURL:    thumbnailURL,
		TargetURL:       targetURL,
		Position:        position,
		ClickTargetType: clickTarget,
		ClickTargetID:   clickTargetID,
		DurationDays:    durationDays,
		IsActive:        false,
	}
}

// validAdPosition folds an untrusted placement onto the five slots the
// storefront actually renders.
func validAdPosition(raw string) string {
	switch raw {
	case promo.PositionHomeHero, promo.PositionHomeBanner, promo.PositionHomeDeals,
		promo.PositionHomeBottom, promo.PositionCatalogTop:
		return raw
	default:
		return promo.PositionHomeHero
	}
}

// validateAdSubmission refuses an advertisement the platform cannot run.
//
// It returns a translated message, or the empty string when the submission is
// acceptable. Everything here was previously enforced only by a disabled button
// in the wizard.
func (h *UIHandler) validateAdSubmission(
	ctx context.Context, orgID int64, ad *promo.Ad, lang string,
) string {
	if ad == nil {
		return i18n.T(lang, "common.invalid_form_data")
	}
	if strings.TrimSpace(ad.TitleAr) == "" {
		return i18n.T(lang, "vendor.ads.title_required")
	}

	// An advertisement must point at something this supplier can ship. The
	// wizard only offers their own in-stock variants; this is what makes that
	// true of the request as well.
	if ad.ClickTargetType == promo.ClickTargetProduct {
		if ad.ClickTargetID == nil || *ad.ClickTargetID <= 0 {
			return i18n.T(lang, "vendor.ads.product_required")
		}
		if h.catSvc == nil {
			return i18n.T(lang, "common.service_unavailable")
		}
		variant, err := h.catSvc.GetVariant(ctx, *ad.ClickTargetID)
		if err != nil || variant == nil || variant.OrganizationID != orgID {
			return i18n.T(lang, "vendor.ads.product_not_yours")
		}
		stockQty := variant.StockQty
		if h.invSvc != nil {
			avail, err := h.invSvc.AvailableQuantity(ctx, *ad.ClickTargetID)
			if err == nil {
				stockQty = avail
			}
		}
		if stockQty <= 0 {
			return i18n.T(lang, "vendor.ads.product_out_of_stock")
		}
	}

	// Credits are deliberately not checked here. CreateAd deducts them inside
	// its own transaction and refuses with ad.insufficient_credits; checking
	// first would be a second answer to the same question, and the two could
	// disagree under concurrency.
	return ""
}
