package ui

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/media"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

type AdminImageImportPhase = pages.AdminImageImportPhase

const (
	AdminImagePhaseUpload     = pages.AdminImagePhaseUpload
	AdminImagePhaseMapping    = pages.AdminImagePhaseMapping
	AdminImagePhaseProcessing = pages.AdminImagePhaseProcessing
	AdminImagePhaseCompleted  = pages.AdminImagePhaseCompleted
	AdminImagePhaseFailed     = pages.AdminImagePhaseFailed
)

type AdminImageImportRow = pages.AdminImageImportRow
type AdminImageImportSession = pages.AdminImageImportSession

type AdminImageImportSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*AdminImageImportSession
}

var globalAdminImageImportSessionStore = &AdminImageImportSessionStore{
	sessions: make(map[string]*AdminImageImportSession),
}

var stopAdminImageImportCleanup = make(chan struct{})

func init() {
	go func() {
		defer func() {
			_ = recover()
		}()
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopAdminImageImportCleanup:
				return
			case <-ticker.C:
				globalAdminImageImportSessionStore.cleanupExpired()
			}
		}
	}()
}

func (s *AdminImageImportSessionStore) NewSession(orgID, userID int64, filename string, totalRows int) *AdminImageImportSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)

	sess := &AdminImageImportSession{
		ID:             id,
		OrgID:          orgID,
		UserID:         userID,
		Filename:       filename,
		Phase:          AdminImagePhaseMapping,
		Progress:       0,
		ProgressNote:   i18n.T("ar", "ops.image.awaiting_columns"),
		TotalRows:      totalRows,
		IdentifierType: "sku",
		DetectedSKUCol: -1,
		DetectedURLCol: -1,
		Rows:           make([]*AdminImageImportRow, 0, totalRows),
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(4 * time.Hour),
	}
	s.sessions[id] = sess
	return sess
}

func (s *AdminImageImportSessionStore) GetSession(id string) (*AdminImageImportSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

func (s *AdminImageImportSessionStore) ListSessions() []*AdminImageImportSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*AdminImageImportSession, 0, len(s.sessions))
	now := time.Now()
	for _, sess := range s.sessions {
		if now.Before(sess.ExpiresAt) {
			list = append(list, sess)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list
}

func (s *AdminImageImportSessionStore) DeleteSession(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; ok {
		delete(s.sessions, id)
		return true
	}
	return false
}

func (s *AdminImageImportSessionStore) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}

// ProcessImageImport runs the background image download and linking pipeline.
func (s *AdminImageImportSessionStore) ProcessImageImport(
	ctx context.Context,
	sessionID string,
	skuCol int,
	urlCol int,
	catSvc *catalog.Service,
	storageClient *storage.Client,
) {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
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
	s.mu.Unlock()

	prodUploadDir := filepath.Join(UploadBaseDir, "products")
	_ = os.MkdirAll(prodUploadDir, 0755)

	total := len(rawRows)
	var successCount, notFoundCount, errorCount int
	outRows := make([]*AdminImageImportRow, 0, total)

	for i, raw := range rawRows {
		rowIdx := i + 1
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
			errorCount++
			outRows = append(outRows, itemRow)
			continue
		}

		if urlVal == "" || (!strings.HasPrefix(urlVal, "http://") && !strings.HasPrefix(urlVal, "https://")) {
			itemRow.Status = "invalid_url"
			itemRow.ErrorMsg = i18n.T("ar", "ops.image.url_invalid")
			errorCount++
			outRows = append(outRows, itemRow)
			continue
		}

		// Look up product in master catalog by selected identifier type (barcode or sku)
		var prod *catalog.Product
		var err error
		if idType == "barcode" {
			prod, err = catSvc.GetProductByBarcode(ctx, skuVal)
		} else {
			prod, err = catSvc.GetProductBySKU(ctx, skuVal)
		}
		if err != nil || prod == nil {
			itemRow.Status = "not_found"
			if idType == "barcode" {
				itemRow.ErrorMsg = fmt.Sprintf("الصنف ذو الباركود «%s» غير مسجل في الكتالوج المعتمد", skuVal)
			} else {
				itemRow.ErrorMsg = fmt.Sprintf(i18n.T("ar", "ops.image.prod_not_found"), skuVal)
			}
			notFoundCount++
			outRows = append(outRows, itemRow)
			continue
		}

		itemRow.ProductID = &prod.ID
		prodName := prod.Name.Get("ar")
		if prodName == "" {
			prodName = prod.Name.Get("en")
		}
		itemRow.ProductName = prodName

		// Download image safely
		imgData, ext, dlErr := ingest.DownloadProductImage(ctx, urlVal)
		if dlErr != nil {
			itemRow.Status = "download_failed"
			itemRow.ErrorMsg = fmt.Sprintf(i18n.TDefault("w4_ui.v_9"), dlErr)
			errorCount++
			outRows = append(outRows, itemRow)
			continue
		}

		// Apply subtle Dawa24 watermark to the image
		if watermarked, wmErr := ingest.ApplyWatermark(imgData, ext); wmErr == nil && len(watermarked) > 0 {
			imgData = watermarked
		}

		// Compress and optimize image for storage
		if compBytes, compExt, _, wasCompressed := media.Compress(imgData, media.DefaultMaxEdge); wasCompressed {
			imgData = compBytes
			ext = strings.TrimPrefix(compExt, ".")
		}

		// Save locally to data/uploads/products/<uuid>.<ext>
		fileName := fmt.Sprintf("%s.%s", uuid.New().String(), ext)
		localPath := filepath.Join(prodUploadDir, fileName)
		if err := os.WriteFile(localPath, imgData, 0644); err != nil {
			itemRow.Status = "download_failed"
			itemRow.ErrorMsg = i18n.TDefault("w4_ui.s_7_7")
			errorCount++
			outRows = append(outRows, itemRow)
			continue
		}

		publicPath := fmt.Sprintf("/uploads/products/%s", fileName)

		// Also upload to S3 if storage client is configured
		if storageClient != nil {
			s3Key := fmt.Sprintf("products/%s", fileName)
			_ = storageClient.Put(ctx, s3Key, bytes.NewReader(imgData), int64(len(imgData)), "image/"+ext)
		}

		// Update product in catalog database
		updatedProd, updateErr := catSvc.UpdateProductImageByID(ctx, prod.ID, publicPath, urlVal)
		if updateErr != nil {
			itemRow.Status = "download_failed"
			itemRow.ErrorMsg = fmt.Sprintf(i18n.TDefault("w4_ui.v_10"), updateErr)
			errorCount++
			outRows = append(outRows, itemRow)
			continue
		}

		itemRow.Status = "success"
		itemRow.SavedImagePath = publicPath
		if updatedProd != nil {
			itemRow.ProductID = &updatedProd.ID
		}
		successCount++
		outRows = append(outRows, itemRow)

		// Progress update
		if (i+1)%5 == 0 || (i+1) == total {
			s.mu.Lock()
			if curSess, ok := s.sessions[sessionID]; ok {
				curSess.Progress = int(float64(i+1) / float64(total) * 100)
				curSess.ProgressNote = fmt.Sprintf(i18n.TDefault("w4_ui.d_d_11"), i+1, total)
				curSess.SuccessRows = successCount
				curSess.NotFoundRows = notFoundCount
				curSess.ErrorRows = errorCount
				curSess.Rows = outRows
			}
			s.mu.Unlock()
		}
	}

	s.mu.Lock()
	if curSess, ok := s.sessions[sessionID]; ok {
		curSess.Phase = AdminImagePhaseCompleted
		curSess.Progress = 100
		curSess.ProgressNote = i18n.TDefault("w4_ui.w4str_12_12")
		curSess.SuccessRows = successCount
		curSess.NotFoundRows = notFoundCount
		curSess.ErrorRows = errorCount
		curSess.Rows = outRows
	}
	s.mu.Unlock()
}
