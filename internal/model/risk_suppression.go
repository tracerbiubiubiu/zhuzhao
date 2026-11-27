package model

import "time"

// RiskFalsePositiveRequest closes an unlinked risk and records a bounded family suppression.
type RiskFalsePositiveRequest struct {
	ID                    int64     `json:"id,string" binding:"required"`
	OperationID           string    `json:"operation_id" binding:"required,max=128"`
	ExpectedRevision      string    `json:"expected_revision" binding:"required"`
	Reason                string    `json:"reason" binding:"required,max=2000"`
	SuppressionValidUntil time.Time `json:"suppression_valid_until" binding:"required"`
}

type RiskSuppressionRemoveRequest struct {
	ID               int64  `json:"id,string" binding:"required"`
	OperationID      string `json:"operation_id" binding:"required,max=128"`
	ExpectedRevision string `json:"expected_revision" binding:"required"`
	Reason           string `json:"reason" binding:"required,max=2000"`
}

type RiskSuppressionRef struct {
	ID         int64     `json:"id,string"`
	Revision   int64     `json:"revision,string"`
	ValidUntil time.Time `json:"valid_until"`
}

type RiskSuppressionActionResult struct {
	Risk        RiskRef            `json:"risk"`
	Status      string             `json:"status"`
	Suppression RiskSuppressionRef `json:"suppression"`
	Receipt     RiskReceipt        `json:"receipt_ref"`
}

type RiskSuppressionView struct {
	RiskSuppressionRef
	RiskID      int64      `json:"risk_id,string"`
	Reason      string     `json:"reason"`
	CreatedBy   int64      `json:"created_by,string"`
	CreatedAt   time.Time  `json:"created_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	EndedBy     *int64     `json:"ended_by,string,omitempty"`
	EndedReason *string    `json:"ended_reason,omitempty"`
	Active      bool       `json:"active"`
}

type RiskSuppressionList struct {
	Items    []RiskSuppressionView `json:"items"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
}
