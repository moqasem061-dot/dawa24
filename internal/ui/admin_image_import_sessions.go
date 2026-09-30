package ui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/storage"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
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

func (s *AdminImageImportSessionStore) NewSession(initial AdminImageImportSession) *AdminImageImportSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)

	sess := &AdminImageImportSession{
		ID:             id,
		OrgID:          initial.OrgID,
		UserID:         initial.UserID,
		Filename:       initial.Filename,
		Phase:          AdminImagePhaseMapping,
		Progress:       0,
		ProgressNote:   i18n.T("ar", "ops.image.awaiting_columns"),
		TotalRows:      initial.TotalRows,
		IdentifierType: "sku",
		Headers:        initial.Headers,
		DetectedSKUCol: initial.DetectedSKUCol,
		DetectedURLCol: initial.DetectedURLCol,
		SampleRows:     initial.SampleRows,
		RawDataRows:    initial.RawDataRows,
		Rows:           make([]*AdminImageImportRow, 0, initial.TotalRows),
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(4 * time.Hour),
	}
	s.sessions[id] = sess
	return snapshotAdminImageSession(sess)
}

func (s *AdminImageImportSessionStore) GetSession(id string) (*AdminImageImportSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return snapshotAdminImageSession(sess), true
}

func (s *AdminImageImportSessionStore) ListSessions() []*AdminImageImportSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*AdminImageImportSession, 0, len(s.sessions))
	now := time.Now()
	for _, sess := range s.sessions {
		if now.Before(sess.ExpiresAt) {
			list = append(list, snapshotAdminImageSession(sess))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list
}

// Sheet data and completed rows are immutable; progress and row slices are snapshots.
func snapshotAdminImageSession(sess *AdminImageImportSession) *AdminImageImportSession {
	snapshot := *sess
	snapshot.Rows = slices.Clone(sess.Rows)
	return &snapshot
}

func (s *AdminImageImportSessionStore) SetMapping(id, identifierType string, skuCol, urlCol int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return false
	}
	sess.IdentifierType = identifierType
	sess.DetectedSKUCol = skuCol
	sess.DetectedURLCol = urlCol
	return true
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

// ProcessImageImport runs the background image download and linking pipeline concurrently.
func (s *AdminImageImportSessionStore) ProcessImageImport(
	ctx context.Context,
	sessionID string,
	skuCol int,
	urlCol int,
	catSvc *catalog.Service,
	storageClient *storage.Client,
) {
	runConcurrentImageImport(ctx, s, sessionID, skuCol, urlCol, catSvc, storageClient)
}
