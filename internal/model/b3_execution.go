package model

import (
	"encoding/json"
	"time"
)

// TicketTaskRun is the original node execution, visible through ticket detail.
type TicketTaskRun struct {
	OperationID    string          `json:"operation_id"`
	TaskID         string          `json:"task_id"`
	Action         string          `json:"action"`
	TicketRevision int64           `json:"ticket_revision,string"`
	DefinitionHash string          `json:"definition_hash"`
	Node           string          `json:"node"`
	Status         string          `json:"status"`
	Reason         string          `json:"reason,omitempty"`
	Result         json.RawMessage `json:"result,omitempty" swaggertype:"object"`
	CreatedAt      time.Time       `json:"created_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
}
