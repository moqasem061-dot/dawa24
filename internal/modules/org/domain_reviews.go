package org

// AdminReviewFilter holds multi-dimensional filters for querying platform vendor reviews.
type AdminReviewFilter struct {
	VendorOrgID   *int64
	ReviewerOrgID *int64
	Rating        *int
	Status        string // "approved", "pending", "all"
	Search        string // matching review text, order number, vendor name, reviewer name
	Limit         int
	Offset        int
}

// AdminReviewStats holds aggregate KPIs for platform reviews.
type AdminReviewStats struct {
	TotalReviews  int     `json:"total_reviews"`
	ApprovedCount int     `json:"approved_count"`
	PendingCount  int     `json:"pending_count"`
	AverageRating float64 `json:"average_rating"`
	AvgScoreRep   float64 `json:"avg_score_rep"`
	AvgScoreQual  float64 `json:"avg_score_qual"`
	AvgScoreSpeed float64 `json:"avg_score_speed"`
}
