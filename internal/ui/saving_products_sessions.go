package ui

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

type SavingImportSession = pages.SavingImportSession
type SavingRowFilter = pages.SavingRowFilter
type StagedSavingItem = pages.StagedSavingItem
type SavingDetectedCols = pages.SavingDetectedCols
type SavingImportPhase = pages.SavingImportPhase
type SessionState = pages.SessionState

const (
	SavingPhaseUpload    = pages.SavingPhaseUpload
	SavingPhaseMapping   = pages.SavingPhaseMapping
	SavingPhaseReview    = pages.SavingPhaseReview
	SavingPhaseCompleted = pages.SavingPhaseCompleted

	SessionStateUploaded   = pages.SessionStateUploaded
	SessionStateProcessing = pages.SessionStateProcessing
	SessionStateReady      = pages.SessionStateReady
	SessionStateCommitted  = pages.SessionStateCommitted
	SessionStateCancelled  = pages.SessionStateCancelled
	SessionStateFailed     = pages.SessionStateFailed
)

// SavingImportSessionStore is an in-memory thread-safe store for async staging sessions.
type SavingImportSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*SavingImportSession
}

var globalSavingImportSessionStore = &SavingImportSessionStore{
	sessions: make(map[string]*SavingImportSession),
}

// GlobalSavingImportSessionStore returns the shared saving import session store.
func GlobalSavingImportSessionStore() *SavingImportSessionStore {
	return globalSavingImportSessionStore
}

var stopSavingImportCleanup = make(chan struct{})

func init() {
	go func() {
		defer func() {
			_ = recover()
		}()
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopSavingImportCleanup:
				return
			case <-ticker.C:
				globalSavingImportSessionStore.cleanupExpired()
			}
		}
	}()
}

// NewSession creates a new session in processing state.
func (s *SavingImportSessionStore) NewSession(orgID, userID int64, filename string, totalRows int) *SavingImportSession {
	return s.NewSessionWithID(uuid.NewString(), orgID, userID, filename, totalRows)
}

// NewSessionWithID creates a new session in processing state with a predetermined ID.
//
// "Processing" here means a background goroutine is already matching the rows —
// this constructor is for the async run only. A session that is still waiting
// for the buyer to confirm its columns must be created with NewMappingSession,
// or the wizard skips straight past the mapping screen. See the comment there.
func (s *SavingImportSessionStore) NewSessionWithID(id string, orgID, userID int64, filename string, totalRows int) *SavingImportSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := &SavingImportSession{
		Success:       true,
		ID:            id,
		OrgID:         orgID,
		UserID:        userID,
		Filename:      filename,
		Status:        SessionStateProcessing,
		Phase:         SavingPhaseReview,
		Progress:      5,
		ProgressPhase: i18n.TDefault("w4_ui.s_94_94"),
		TotalRows:     totalRows,
		Items:         make([]*StagedSavingItem, 0, totalRows),
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(4 * time.Hour),
	}

	s.sessions[id] = session
	return session
}

// NewMappingSession creates a session that has been parsed and is waiting for
// the buyer to confirm its columns.
//
// It exists because the upload handlers used to call NewSession — which marks a
// session `processing`, for the async run — and then reassign Phase to mapping
// afterwards. Two things went wrong with that, and together they are the whole
// "stuck at 4%" report:
//
//  1. The wizard tests Status before it tests Phase, so a session left
//     `processing` rendered the matching screen and the mapping screen was
//     never reachable. Step 2 of the wizard was silently skipped.
//  2. Nothing was matching. The goroutine that moves the bar is started by the
//     mapping form, which the buyer never got to submit. So the bar polled a
//     session frozen at Progress 5, and the client-side drift floors that to
//     the 4% that was reported — forever.
//
// The fields are set under the same lock that publishes the session, rather
// than mutated by the caller afterwards, because ListSessions and the progress
// endpoint read them from other goroutines.
func (s *SavingImportSessionStore) NewMappingSession(
	orgID, userID int64,
	filename string,
	headers []string,
	sampleRows, dataRows [][]string,
	detected SavingDetectedCols,
) *SavingImportSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.NewString()
	session := &SavingImportSession{
		Success:       true,
		ID:            id,
		OrgID:         orgID,
		UserID:        userID,
		Filename:      filename,
		Status:        SessionStateUploaded,
		Phase:         SavingPhaseMapping,
		Progress:      0,
		ProgressPhase: "",
		TotalRows:     len(dataRows),
		Headers:       headers,
		SampleRows:    sampleRows,
		RawDataRows:   dataRows,
		DetectedCols:  detected,
		Items:         make([]*StagedSavingItem, 0, len(dataRows)),
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(4 * time.Hour),
	}

	s.sessions[id] = session
	return session
}

// ListSessions returns all active sessions of an organization, sorted newest first.
func (s *SavingImportSessionStore) ListSessions(orgID int64) []*SavingImportSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*SavingImportSession
	for _, sess := range s.sessions {
		if sess.OrgID == orgID && time.Now().Before(sess.ExpiresAt) {
			list = append(list, sess)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list
}

// GetSession retrieves a session by ID with ownership check.
func (s *SavingImportSessionStore) GetSession(id string, orgID int64) (*SavingImportSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[id]
	if !ok || sess.OrgID != orgID || time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

// GetSessionForAdmin retrieves an active session by ID without tenant ownership restriction for platform staff.
func (s *SavingImportSessionStore) GetSessionForAdmin(id string) (*SavingImportSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[id]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

func (s *SavingImportSessionStore) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}
