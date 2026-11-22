package model

type TicketActionCollectField struct {
	Key      string `json:"key"`
	Required bool   `json:"required"`
}

type TicketAvailableAction struct {
	ActionID         string                     `json:"action_id"`
	To               string                     `json:"to"`
	Label            string                     `json:"label"`
	CollectFields    []TicketActionCollectField `json:"collect_fields"`
	CommentRequired  bool                       `json:"comment_required"`
	ExpectedRevision int64                      `json:"expected_revision,string"`
	DefinitionRef    *TicketDefinitionRef       `json:"definition_ref"`
}

type TicketActionList struct {
	Actions []TicketAvailableAction    `json:"actions"`
	Fields  []PublishedTicketFormField `json:"fields"`
}
