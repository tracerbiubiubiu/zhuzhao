package model

import "time"

// RiskWorkflowContext presents the current revisions required to work on each
// visible asset of a risk-linked ticket.
type RiskWorkflowContext struct {
	TicketID          int64              `json:"ticket_id,string"`
	TicketRevision    int64              `json:"ticket_revision,string"`
	DefinitionVersion int64              `json:"definition_version,string"`
	Status            string             `json:"status"`
	WritesEnabled     bool               `json:"writes_enabled"`
	AllowedActions    []string           `json:"allowed_actions"`
	Items             []RiskWorkflowItem `json:"items"`
}

type RiskWorkflowItem struct {
	LinkID                int64                    `json:"link_id,string"`
	RiskID                int64                    `json:"risk_id,string"`
	RiskRevision          int64                    `json:"risk_revision,string"`
	LinkRevision          int64                    `json:"link_revision,string"`
	ProposalVersion       int64                    `json:"proposal_version,string"`
	RemediationState      string                   `json:"remediation_state"`
	Title                 string                   `json:"title"`
	IsRiskOwner           bool                     `json:"is_risk_owner"`
	CanManageRisk         bool                     `json:"can_manage_risk"`
	AcceptanceID          int64                    `json:"pending_acceptance_id,string,omitempty"`
	AcceptanceRevision    int64                    `json:"pending_acceptance_revision,string,omitempty"`
	AcceptanceRecoverable bool                     `json:"pending_acceptance_recoverable,omitempty"`
	Submissions           []RiskWorkflowSubmission `json:"submissions,omitempty"`
}

type RiskWorkflowSubmission struct {
	Version     int64                  `json:"proposal_version,string"`
	Measures    string                 `json:"measures"`
	Evidence    []RiskWorkflowEvidence `json:"evidence"`
	SubmittedAt time.Time              `json:"submitted_at"`
	Decision    *string                `json:"decision,omitempty"`
	ReviewNote  *string                `json:"review_note,omitempty"`
	ReviewedAt  *time.Time             `json:"reviewed_at,omitempty"`
}

type RiskWorkflowEvidence struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Version string `json:"version"`
}
