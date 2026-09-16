package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/media"
)

// watermarkImportedImages looks up committable rows from the session that provided
// an external image URL, downloads each image, applies the watermark, saves it
// to upload storage, and updates the product catalog record.
func (h *UIHandler) watermarkImportedImages(sessionID int64) {
	if h.catSvc == nil {
		return
	}

	h.safeGo("admin-watermark-imported-images", func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		rows, err := h.catSvc.LoadCommittableRows(bgCtx, sessionID)
		if err != nil || len(rows) == 0 {
			return
		}

		var targets []struct {
			SKU string
			URL string
		}
		for _, row := range rows {
			if row.Product == nil {
				continue
			}
			imgURL := strings.TrimSpace(row.Product.ImageLink)
			if imgURL == "" {
				imgURL = strings.TrimSpace(row.Product.Image)
			}
			sku := strings.TrimSpace(row.Product.SKU)
			if (strings.HasPrefix(imgURL, "http://") || strings.HasPrefix(imgURL, "https://")) && sku != "" {
				targets = append(targets, struct {
					SKU string
					URL string
				}{SKU: sku, URL: imgURL})
			}
		}

		if len(targets) == 0 {
			return
		}

		prodUploadDir := filepath.Join(GetUploadBaseDir(), "products")
		_ = os.MkdirAll(prodUploadDir, 0755)

		for _, item := range targets {
			imgData, ext, dlErr := ingest.DownloadProductImage(bgCtx, item.URL)
			if dlErr != nil || len(imgData) == 0 {
				continue
			}

			// Apply watermark
			if watermarked, wmErr := ingest.ApplyWatermark(imgData, ext); wmErr == nil && len(watermarked) > 0 {
				imgData = watermarked
			}

			// Compress and optimize image for storage
			if compBytes, compExt, _, wasCompressed := media.Compress(imgData, media.DefaultMaxEdge); wasCompressed {
				imgData = compBytes
				ext = strings.TrimPrefix(compExt, ".")
			}

			fileName := fmt.Sprintf("%s.%s", uuid.New().String(), ext)
			localPath := filepath.Join(prodUploadDir, fileName)
			if err := os.WriteFile(localPath, imgData, 0644); err != nil {
				continue
			}

			publicPath := fmt.Sprintf("/uploads/products/%s", fileName)
			if h.storage != nil {
				s3Key := fmt.Sprintf("products/%s", fileName)
				_ = h.storage.Put(bgCtx, s3Key, bytes.NewReader(imgData), int64(len(imgData)), "image/"+ext)
			}

			_, _ = h.catSvc.UpdateProductImageBySKU(bgCtx, item.SKU, publicPath, item.URL)
		}
	})
}

// refreshProductIndex rebuilds the denormalised search table after an import.
func (h *UIHandler) refreshProductIndex(ctx context.Context) {
	if h.catSvc == nil {
		return
	}
	// Detached from the request: the admin should not wait on it, and a client
	// disconnect must not abort a rebuild that is already underway.
	go func() {
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		defer func() {
			if p := recover(); p != nil {
				h.log.ErrorContext(bg, "rebuild product index panicked", "panic", p)
			}
		}()
		count, err := h.catSvc.RebuildProductIndex(database.AsSystem(bg))
		if err != nil {
			h.log.ErrorContext(bg, "rebuild product index after import", "error", err)
			return
		}
		h.log.InfoContext(bg, "product index rebuilt after import", "rows", count)
	}()
}
