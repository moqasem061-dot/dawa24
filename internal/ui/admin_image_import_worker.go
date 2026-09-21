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
	"github.com/muhiya/dawa24-store/internal/platform/media"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// urlDownloadCache coordinates deduplicated concurrent image fetching.
type urlDownloadCache struct {
	mu      sync.Mutex
	inProg  map[string]*urlDownloadFlight
	fetcher func(ctx context.Context, imageURL, uploadDir string, storageClient *storage.Client) (string, error)
}

type urlDownloadFlight struct {
	done chan struct{}
	path string
	err  error
}

var globalImageDownloadCache = &urlDownloadCache{
	inProg: make(map[string]*urlDownloadFlight),
}

func (c *urlDownloadCache) FetchOrDownload(
	ctx context.Context,
	rawURL string,
	uploadDir string,
	storageClient *storage.Client,
) (string, error) {
	cleanURL := strings.TrimSpace(rawURL)
	c.mu.Lock()
	if flight, exists := c.inProg[cleanURL]; exists {
		c.mu.Unlock()
		select {
		case <-flight.done:
			return flight.path, flight.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	flight := &urlDownloadFlight{
		done: make(chan struct{}),
	}
	c.inProg[cleanURL] = flight
	c.mu.Unlock()

	dlFunc := c.fetcher
	if dlFunc == nil {
		dlFunc = downloadSingleImage
	}
	path, err := dlFunc(ctx, cleanURL, uploadDir, storageClient)
	flight.path = path
	flight.err = err
	close(flight.done)

	return path, err
}

func downloadSingleImage(
	ctx context.Context,
	imageURL string,
	uploadDir string,
	storageClient *storage.Client,
) (string, error) {
	dlCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	imgData, ext, dlErr := ingest.DownloadProductImage(dlCtx, imageURL)
	if dlErr != nil {
		return "", dlErr
	}
	if len(imgData) == 0 {
		return "", fmt.Errorf("empty image payload received")
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
		return "", fmt.Errorf("failed writing image to disk: %w", err)
	}

	_ = deriveAndStore(uploadDir, fileName, imgData)

	publicPath := fmt.Sprintf("/uploads/products/%s", fileName)

	if storageClient != nil {
		s3Key := fmt.Sprintf("products/%s", fileName)
		_ = storageClient.Put(ctx, s3Key, bytes.NewReader(imgData), int64(len(imgData)), "image/"+ext)
	}

	return publicPath, nil
}

type imageImportJob struct {
	index  int
	rawRow []string
}

func runConcurrentImageImport(
	ctx context.Context,
	store *AdminImageImportSessionStore,
	sessionID string,
	skuCol int,
	urlCol int,
	catSvc *catalog.Service,
	storageClient *storage.Client,
) {
	store.mu.Lock()
	sess, ok := store.sessions[sessionID]
	if !ok {
		store.mu.Unlock()
		return
	}
	sess.Phase = AdminImagePhaseProcessing
	sess.Progress = 0
	sess.ProgressNote = i18n.T("ar", "ops.image.start_download")
	rawRows := sess.RawDataRows
	idType := sess.IdentifierType
	if idType == "" {
		idType = "sku"
	}
	store.mu.Unlock()

	prodUploadDir := filepath.Join(GetUploadBaseDir(), "products")
	_ = os.MkdirAll(prodUploadDir, 0755)

	total := len(rawRows)
	if total == 0 {
		store.mu.Lock()
		if curSess, ok := store.sessions[sessionID]; ok {
			curSess.Phase = AdminImagePhaseCompleted
			curSess.Progress = 100
		}
		store.mu.Unlock()
		return
	}

	outRows := make([]*AdminImageImportRow, total)
	var (
		statsMu        sync.Mutex
		completedCount int
		successCount   int
		notFoundCount  int
		errorCount     int
	)

	concurrency := 12
	if total < concurrency {
		concurrency = total
	}

	jobs := make(chan imageImportJob, concurrency*2)
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				itemRow := processOneImageRow(
					ctx, job.index, job.rawRow, skuCol, urlCol, idType,
					catSvc, storageClient, prodUploadDir,
				)

				statsMu.Lock()
				outRows[job.index-1] = itemRow
				completedCount++
				switch itemRow.Status {
				case "success":
					successCount++
				case "not_found":
					notFoundCount++
				default:
					errorCount++
				}

				if completedCount%5 == 0 || completedCount == total {
					store.mu.Lock()
					if curSess, ok := store.sessions[sessionID]; ok {
						curSess.Progress = int(float64(completedCount) / float64(total) * 100)
						curSess.ProgressNote = fmt.Sprintf(i18n.TDefault("w4_ui.d_d_11"), completedCount, total)
						curSess.SuccessRows = successCount
						curSess.NotFoundRows = notFoundCount
						curSess.ErrorRows = errorCount
						curSess.Rows = outRows
					}
					store.mu.Unlock()
				}
				statsMu.Unlock()
			}
		}()
	}

	for i, raw := range rawRows {
		jobs <- imageImportJob{index: i + 1, rawRow: raw}
	}
	close(jobs)
	wg.Wait()

	store.mu.Lock()
	if curSess, ok := store.sessions[sessionID]; ok {
		curSess.Phase = AdminImagePhaseCompleted
		curSess.Progress = 100
		curSess.ProgressNote = i18n.TDefault("w4_ui.w4str_12_12")
		curSess.SuccessRows = successCount
		curSess.NotFoundRows = notFoundCount
		curSess.ErrorRows = errorCount
		curSess.Rows = outRows
	}
	store.mu.Unlock()
}

func processOneImageRow(
	ctx context.Context,
	rowIdx int,
	raw []string,
	skuCol, urlCol int,
	idType string,
	catSvc *catalog.Service,
	storageClient *storage.Client,
	uploadDir string,
) *AdminImageImportRow {
	var skuVal, urlVal string
	if skuCol >= 0 && skuCol < len(raw) {
		skuVal = strings.TrimSpace(raw[skuCol])
	}
	if urlCol >= 0 && urlCol < len(raw) {
		urlVal = strings.TrimSpace(raw[urlCol])
	}

	itemRow := &AdminImageImportRow{
		Index:    rowIdx,
		SKU:      skuVal,
		ImageURL: urlVal,
		Status:   "pending",
	}

	if skuVal == "" {
		itemRow.Status = "invalid_url"
		if idType == "barcode" {
			itemRow.ErrorMsg = "لم يتم تحديد باركود الصنف في هذا الصف"
		} else {
			itemRow.ErrorMsg = i18n.T("ar", "ops.image.sku_empty")
		}
		return itemRow
	}

	if urlVal == "" || (!strings.HasPrefix(urlVal, "http://") && !strings.HasPrefix(urlVal, "https://")) {
		itemRow.Status = "invalid_url"
		itemRow.ErrorMsg = i18n.T("ar", "ops.image.url_invalid")
		return itemRow
	}

	var prod *catalog.Product
	var err error
	if idType == "barcode" {
		prod, err = catSvc.GetProductByBarcode(ctx, skuVal)
		if (err != nil || prod == nil) && catSvc != nil {
			if fb, fbErr := catSvc.GetProductBySKU(ctx, skuVal); fbErr == nil && fb != nil {
				prod = fb
				err = nil
			}
		}
	} else {
		prod, err = catSvc.GetProductBySKU(ctx, skuVal)
		if (err != nil || prod == nil) && catSvc != nil {
			if fb, fbErr := catSvc.GetProductByBarcode(ctx, skuVal); fbErr == nil && fb != nil {
				prod = fb
				err = nil
			}
		}
	}

	if err != nil || prod == nil {
		itemRow.Status = "not_found"
		if idType == "barcode" {
			itemRow.ErrorMsg = fmt.Sprintf("الصنف ذو الباركود «%s» غير مسجل في الكتالوج المعتمد", skuVal)
		} else {
			itemRow.ErrorMsg = fmt.Sprintf(i18n.T("ar", "ops.image.prod_not_found"), skuVal)
		}
		return itemRow
	}

	itemRow.ProductID = &prod.ID
	prodName := prod.Name.Get("ar")
	if prodName == "" {
		prodName = prod.Name.Get("en")
	}
	itemRow.ProductName = prodName

	publicPath, dlErr := globalImageDownloadCache.FetchOrDownload(ctx, urlVal, uploadDir, storageClient)
	if dlErr != nil {
		itemRow.Status = "download_failed"
		itemRow.ErrorMsg = fmt.Sprintf(i18n.TDefault("w4_ui.v_9"), dlErr)
		return itemRow
	}

	updatedProd, updateErr := catSvc.UpdateProductImageByID(ctx, prod.ID, publicPath, urlVal)
	if updateErr != nil {
		itemRow.Status = "download_failed"
		itemRow.ErrorMsg = fmt.Sprintf(i18n.TDefault("w4_ui.v_10"), updateErr)
		return itemRow
	}

	itemRow.Status = "success"
	itemRow.SavedImagePath = publicPath
	if updatedProd != nil {
		itemRow.ProductID = &updatedProd.ID
	}
	return itemRow
}
