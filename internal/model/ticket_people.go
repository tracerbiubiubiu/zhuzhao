package model

// TicketPeopleQuery binds a search to one existing ticket write use case.
type TicketPeopleQuery struct {
	Purpose  string `form:"purpose"`
	TicketID int64  `form:"ticket_id"`
	OrgID    int64  `form:"org_id"`
	TypeCode string `form:"type_code"`
	Query    string `form:"q"`
}

// TicketPerson exposes only a selectable identity and a display label.
type TicketPerson struct {
	ID    int64  `json:"id,string"`
	Label string `json:"label"`
}

// TicketRelationCandidate is the display projection for choosing a related ticket.
type TicketRelationCandidate struct {
	ID     int64  `json:"id,string"`
	Title  string `json:"title"`
	Status string `json:"status"`
}
