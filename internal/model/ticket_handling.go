package model

type TicketWithdrawRequest struct {
	ID               string `json:"id" binding:"required"`
	ExpectedRevision string `json:"expected_revision" binding:"required"`
	Comment          string `json:"comment,omitempty"`
}

type TicketTransferRequest struct {
	TicketWithdrawRequest
	ToUserID string `json:"to_user_id" binding:"required"`
}

type TicketHandlingCapabilities struct {
	Transfer bool `json:"transfer"`
	Withdraw bool `json:"withdraw"`
}

type TicketHandlingResult struct {
	ID         int64  `json:"id,string"`
	AssignedTo *int64 `json:"assigned_to,string" extensions:"x-nullable"`
	Revision   int64  `json:"revision,string"`
	Status     string `json:"status"`
}
