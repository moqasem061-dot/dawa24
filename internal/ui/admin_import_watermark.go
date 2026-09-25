package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/media"
)

type catalogImageTarget struct {
	ProductID int64
	SKU       string
	Barcode   string
	URL       string
}

type catalogImageGroup struct {
	URL        string
	ProductIDs []int64
	SKUs       []string
	Barcodes   []string
}

// cleanTargetImageURL trims and normalizes image URLs from imported rows.
func cleanTargetImageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "\"'`")
	if raw == "" {
		return ""
	}
	if strings.ContainsAny(raw, ",\n\r;\t ") {
		parts := strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == ';' || r == '\t' || r == ' '
		})
		for _, p := range parts {
			p = strings.Trim(strings.TrimSpace(p), "\"'`")
			if p != "" {
				raw = p
				break
			}
		}
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(strings.ToLower(raw), "www.") {
		return "https://" + raw
	}
	return raw
}

// extractCommittedImageTargets extracts products with external image URLs directly from the committed batch.
func extractCommittedImageTargets(prods []*catalog.Product) []catalogImageTarget {
	var targets []catalogImageTarget
	for _, p := range prods {
		if p == nil || p.ID <= 0 {
			continue
		}
		imgURL := cleanTargetImageURL(p.ImageLink)
		if imgURL == "" {
			imgURL = cleanTargetImageURL(p.Image)
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
		imgURL := cleanTargetImageURL(row.Product.ImageLink)
		if imgURL == "" {
			imgURL = cleanTargetImageURL(row.Product.Image)
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

// groupImageTargets deduplicates identical image URLs across products into single download tasks.
func groupImageTargets(targets []catalogImageTarget) []*catalogImageGroup {
	byURL := make(map[string]*catalogImageGroup, len(targets)/2+1)
	for _, t := range targets {
		url := cleanTargetImageURL(t.URL)
		if url == "" {
			continue
		}
		g, exists := byURL[url]
		if !exists {
			g = &catalogImageGroup{URL: url}
			byURL[url] = g
		}
		if t.ProductID > 0 {
			g.ProductIDs = append(g.ProductIDs, t.ProductID)
		}
		if t.SKU != "" {
			g.SKUs = append(g.SKUs, t.SKU)
		}
		if t.Barcode != "" {
			g.Barcodes = append(g.Barcodes, t.Barcode)
		}
	}

	groups := make([]*catalogImageGroup, 0, len(byURL))
	for _, g := range byURL {
		groups = append(groups, g)
	}
	return groups
}

// downloadImportedImages safely downloads, compresses, and links external images to products concurrently.
func (h *UIHandler) downloadImportedImages(targets []catalogImageTarget) {
	if h.catSvc == nil || len(targets) == 0 {
		return
	}

	h.safeGo("admin-download-imported-images", func() {
		groups := groupImageTargets(targets)
		if len(groups) == 0 {
			return
		}

		// Calculate realistic timeout: at least 30 minutes, scaling with unique image count up to 6 hours.
		timeout := time.Duration(len(groups)) * 12 * time.Second
		if timeout < 30*time.Minute {
			timeout = 30 * time.Minute
		} else if timeout > 24*time.Hour {
			timeout = 24 * time.Hour
		}

		bgCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		prodUploadDir := filepath.Join(GetUploadBaseDir(), "products")
		_ = os.MkdirAll(prodUploadDir, 0755)

		concurrency := 36
		if len(groups) < concurrency {
			concurrency = len(groups)
		}

		jobs := make(chan *catalogImageGroup, concurrency*2)
		var (
			wg             sync.WaitGroup
			completedCount int64
			successCount   int64
			failCount      int64
			updatedRows    int64
			totalGroups    = int64(len(groups))
		)

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

					publicPath, err := h.fetchAndStoreImage(bgCtx, item.URL, prodUploadDir)
					atomic.AddInt64(&completedCount, 1)

					if err != nil || publicPath == "" {
						atomic.AddInt64(&failCount, 1)
						continue
					}

					atomic.AddInt64(&successCount, 1)
					sysCtx := database.AsSystem(bgCtx)

					// Fast batch database updates
					if len(item.ProductIDs) > 0 {
						_ = h.catSvc.BatchUpdateProductImage(sysCtx, item.ProductIDs, publicPath, item.URL)
						atomic.AddInt64(&updatedRows, int64(len(item.ProductIDs)))
					}
					if len(item.SKUs) > 0 {
						_ = h.catSvc.BatchUpdateProductImageBySKU(sysCtx, item.SKUs, publicPath, item.URL)
					}
					if len(item.Barcodes) > 0 {
						_ = h.catSvc.BatchUpdateProductImageByBarcode(sysCtx, item.Barcodes, publicPath, item.URL)
					}
					_ = h.catSvc.WireProductImageByLink(sysCtx, item.URL, publicPath)

					curCompleted := atomic.LoadInt64(&completedCount)
					if curCompleted%100 == 0 || curCompleted == totalGroups {
						h.log.InfoContext(bgCtx, "download imported images progress",
							"completed", curCompleted,
							"total_unique", totalGroups,
							"success", atomic.LoadInt64(&successCount),
							"failed", atomic.LoadInt64(&failCount),
							"products_updated", atomic.LoadInt64(&updatedRows),
						)
					}
				}
			}()
		}

	feedLoop:
		for _, g := range groups {
			select {
			case <-bgCtx.Done():
				break feedLoop
			case jobs <- g:
			}
		}
		close(jobs)
		wg.Wait()

		h.log.InfoContext(bgCtx, "finished downloading imported product images",
			"total_targets", len(targets),
			"unique_images", totalGroups,
			"succeeded", atomic.LoadInt64(&successCount),
			"failed", atomic.LoadInt64(&failCount),
			"products_updated", atomic.LoadInt64(&updatedRows),
		)
	})
}

// fetchAndStoreImage downloads, watermarks, compresses, and persists a single image to disk and S3.
//
// It runs on a pool of bare worker goroutines, where an unrecovered panic —
// a malformed image tripping a decoder — ends the whole server. A bad image
// costs that image, never the process.
func (h *UIHandler) fetchAndStoreImage(ctx context.Context, rawURL, uploadDir string) (publicPath string, err error) {
	defer func() {
		if p := recover(); p != nil {
			h.log.ErrorContext(ctx, "imported image processing panicked", "url", rawURL, "panic", p)
			publicPath, err = "", fmt.Errorf("image processing panicked: %v", p)
		}
	}()
	dlCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	imgData, ext, dlErr := ingest.DownloadProductImage(dlCtx, rawURL)
	if dlErr != nil || len(imgData) == 0 {
		return "", dlErr
	}

	if watermarked, wmErr := ingest.ApplyWatermark(imgData, ext); wmErr == nil && len(watermarked) > 0 {
		imgData = watermarked
	}

	if compBytes, compExt, _, wasCompressed := media.Compress(imgData, media.DefaultMaxEdge); wasCompressed {
		imgData = compBytes
		ext = strings.TrimPrefix(compExt, ".")
	}

	fileName := fmt.Sprintf("%s.%s", uuid.New().String(), ext)
	localPath := filepath.Join(uploadDir, fileName)
	if err := os.WriteFile(localPath, imgData, 0644); err != nil {
		return "", err
	}

	_ = deriveAndStore(uploadDir, fileName, imgData)

	publicPath = fmt.Sprintf("/uploads/products/%s", fileName)
	if h.storage != nil {
		s3Key := fmt.Sprintf("products/%s", fileName)
		_ = h.storage.Put(ctx, s3Key, bytes.NewReader(imgData), int64(len(imgData)), "image/"+ext)
	}

	return publicPath, nil
}

// BackfillMissingProductImages discovers products with external image links but empty volume paths and queues them.
func (h *UIHandler) BackfillMissingProductImages(ctx context.Context, limit int) (int, error) {
	if h.catSvc == nil {
		return 0, nil
	}
	prods, err := h.catSvc.ListProductsMissingImages(ctx, limit)
	if err != nil || len(prods) == 0 {
		return 0, err
	}
	targets := extractCommittedImageTargets(prods)
	if len(targets) > 0 {
		h.downloadImportedImages(targets)
	}
	return len(targets), nil
}

// TriggerPostImportImageDownloads ensures any imported products with remote image URLs
// have their images downloaded, watermarked, stored on volume, and wired to the database.
func (h *UIHandler) TriggerPostImportImageDownloads(
	ctx context.Context,
	session *catalog.ImportSession,
	committed []*catalog.Product,
) {
	if h.catSvc == nil {
		return
	}
	sysCtx := database.AsSystem(ctx)
	targets := extractCommittedImageTargets(committed)
	if len(targets) == 0 && session != nil {
		targets = h.extractCommittableImageTargets(sysCtx, session.ID)
	}
	if len(targets) == 0 {
		if missing, err := h.catSvc.ListProductsMissingImages(sysCtx, 25000); err == nil && len(missing) > 0 {
			targets = extractCommittedImageTargets(missing)
		}
	}
	if len(targets) > 0 {
		h.downloadImportedImages(targets)
	}
}

// refreshProductIndex rebuilds the denormalised search table after an import.
func (h *UIHandler) refreshProductIndex(ctx context.Context) {
	if h.catSvc == nil {
		return
	}
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
