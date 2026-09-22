package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/media"
)

type catalogImageTarget struct {
	ProductID int64
	SKU       string
	Barcode   string
	URL       string
}

// extractCommittedImageTargets extracts products with external image URLs directly from the committed batch.
func extractCommittedImageTargets(prods []*catalog.Product) []catalogImageTarget {
	var targets []catalogImageTarget
	for _, p := range prods {
		if p == nil || p.ID <= 0 {
			continue
		}
		imgURL := strings.TrimSpace(p.ImageLink)
		if imgURL == "" {
			imgURL = strings.TrimSpace(p.Image)
		}
		if strings.HasPrefix(imgURL, "http://") || strings.HasPrefix(imgURL, "https://") {
			targets = append(targets, catalogImageTarget{
				ProductID: p.ID,
				SKU:       strings.TrimSpace(p.SKU),
				Barcode:   strings.TrimSpace(p.Barcode),
				URL:       imgURL,
			})
		}
	}
	return targets
}

// extractCommittableImageTargets extracts products with external image URLs before staging rows are wiped.
func (h *UIHandler) extractCommittableImageTargets(ctx context.Context, sessionID int64) []catalogImageTarget {
	if h.catSvc == nil || sessionID <= 0 {
		return nil
	}
	rows, err := h.catSvc.LoadCommittableRows(ctx, sessionID)
	if err != nil || len(rows) == 0 {
		return nil
	}
	var targets []catalogImageTarget
	for _, row := range rows {
		if row.Product == nil {
			continue
		}
		imgURL := strings.TrimSpace(row.Product.ImageLink)
		if imgURL == "" {
			imgURL = strings.TrimSpace(row.Product.Image)
		}
		sku := strings.TrimSpace(row.Product.SKU)
		barcode := strings.TrimSpace(row.Product.Barcode)
		var prodID int64
		if row.Product.ID > 0 {
			prodID = row.Product.ID
		} else if row.MatchedProductID != nil {
			prodID = *row.MatchedProductID
		}
		if (strings.HasPrefix(imgURL, "http://") || strings.HasPrefix(imgURL, "https://")) && (sku != "" || barcode != "" || prodID > 0) {
			targets = append(targets, catalogImageTarget{
				ProductID: prodID,
				SKU:       sku,
				Barcode:   barcode,
				URL:       imgURL,
			})
		}
	}
	return targets
}

// downloadImportedImages safely downloads, compresses, and links external images to products concurrently.
func (h *UIHandler) downloadImportedImages(targets []catalogImageTarget) {
	if h.catSvc == nil || len(targets) == 0 {
		return
	}

	h.safeGo("admin-download-imported-images", func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()

		prodUploadDir := filepath.Join(GetUploadBaseDir(), "products")
		_ = os.MkdirAll(prodUploadDir, 0755)

		concurrency := 5
		jobs := make(chan catalogImageTarget, len(targets))
		for _, t := range targets {
			jobs <- t
		}
		close(jobs)

		var wg sync.WaitGroup
		for w := 0; w < concurrency; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range jobs {
					select {
					case <-bgCtx.Done():
						return
					default:
					}

					imgData, ext, dlErr := ingest.DownloadProductImage(bgCtx, item.URL)
					if dlErr != nil || len(imgData) == 0 {
						continue
					}

					if watermarked, wmErr := ingest.ApplyWatermark(imgData, ext); wmErr == nil && len(watermarked) > 0 {
						imgData = watermarked
					}

					if compBytes, compExt, _, wasCompressed := media.Compress(imgData, media.DefaultMaxEdge); wasCompressed {
						imgData = compBytes
						ext = strings.TrimPrefix(compExt, ".")
					}

					fileName := fmt.Sprintf("%s.%s", uuid.New().String(), ext)
					localPath := filepath.Join(prodUploadDir, fileName)
					if err := os.WriteFile(localPath, imgData, 0644); err != nil {
						continue
					}

					_ = deriveAndStore(prodUploadDir, fileName, imgData)

					publicPath := fmt.Sprintf("/uploads/products/%s", fileName)
					if h.storage != nil {
						s3Key := fmt.Sprintf("products/%s", fileName)
						_ = h.storage.Put(bgCtx, s3Key, bytes.NewReader(imgData), int64(len(imgData)), "image/"+ext)
					}

					sysCtx := database.AsSystem(bgCtx)
					updated := false
					if item.ProductID > 0 {
						if _, err := h.catSvc.UpdateProductImageByID(sysCtx, item.ProductID, publicPath, item.URL); err == nil {
							updated = true
						}
					}
					if !updated && item.SKU != "" {
						if _, err := h.catSvc.UpdateProductImageBySKU(sysCtx, item.SKU, publicPath, item.URL); err == nil {
							updated = true
						}
					}
					if !updated && item.Barcode != "" {
						_, _ = h.catSvc.UpdateProductImageByBarcode(sysCtx, item.Barcode, publicPath, item.URL)
					}
				}
			}()
		}
		wg.Wait()
		h.log.InfoContext(bgCtx, "finished downloading imported product images", "count", len(targets))
	})
}

// watermarkImportedImages looks up committable rows from the session that provided
// an external image URL, downloads each image, applies the watermark, saves it
// to upload storage, and updates the product catalog record.
func (h *UIHandler) watermarkImportedImages(sessionID int64) {
	if h.catSvc == nil || sessionID <= 0 {
		return
	}
	sysCtx := database.AsSystem(context.Background())
	targets := h.extractCommittableImageTargets(sysCtx, sessionID)
	if len(targets) > 0 {
		h.downloadImportedImages(targets)
	}
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
