package erp

import "time"

const (
	KeyPrefixLen               = 8
	DefaultRatePerMinute       = 600
	MinRatePerMinute           = 10
	MaxRatePerMinute           = 6000
	SyncBatchesPerMinute       = 6
	AuthFailuresPerIPPerMinute = 30
	MaxBatchItems              = 50000
	MaxBodyBytes               = 32 << 20
	MaxInflatedBytes           = 128 << 20
	SyncChunkSize              = 2500
	SyncSnoozeSeconds          = 15
	PageDefault                = 100
	PageMax                    = 500
	SyncItemsPageDefault       = 1000
	SyncItemsPageMax           = 5000
	AckMax                     = 500
	PurchaseLinesMax           = 200
	OffersMax                  = 50
	RotateGraceDefault         = 24 * time.Hour
	RotateGraceMax             = 72 * time.Hour
	LastUsedTouchEvery         = time.Minute
	IdempotencyStaleAfter      = 2 * time.Minute
	RetentionAPICalls          = 30 * 24 * time.Hour
	RetentionSyncItems         = 7 * 24 * time.Hour
	RetentionSyncBatches       = 90 * 24 * time.Hour
	RetentionReview            = 180 * 24 * time.Hour
	RetentionMatchRuns         = 30 * 24 * time.Hour
	CleanupBatchRows           = 10000
)
