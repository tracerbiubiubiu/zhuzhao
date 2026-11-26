package model

// IAMReceipt describes the committed local command outcome.
type IAMReceipt struct {
	OperationID string `json:"operation_id"`
	Command     string `json:"command"`
	Target      int64  `json:"target_id,string"`
	Status      string `json:"status"`
	Changed     bool   `json:"changed"`
}
