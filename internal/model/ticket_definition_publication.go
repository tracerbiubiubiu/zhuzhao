package model

import "encoding/json"

type TicketDefinitionPlanRequest struct {
	Code                     string  `json:"code" binding:"required,max=50"`
	ExpectedDraftRevision    string  `json:"expected_draft_revision" binding:"required"`
	ExpectedPublishedVersion *string `json:"expected_published_version"`
}

type TicketDefinitionPublishRequest struct {
	TicketDefinitionPlanRequest
	PlanID                 string   `json:"plan_id" binding:"required"`
	CanonicalHash          string   `json:"canonical_hash" binding:"required"`
	AcknowledgedWarningIDs []string `json:"acknowledged_warning_ids"`
}

type TicketDefinitionRef struct {
	TypeCode          string `json:"type_code"`
	DefinitionVersion int64  `json:"definition_version,string"`
	DefinitionHash    string `json:"definition_hash"`
}

type TicketDefinitionWarning struct {
	ID       string `json:"id"`
	FieldKey string `json:"field_key"`
	Message  string `json:"message"`
}

type TicketDefinitionReferenceRevision struct {
	ID      int64 `json:"id,string"`
	Version int64 `json:"version,string"`
}

type TicketDefinitionPlan struct {
	PlanID             int64                                        `json:"plan_id,string"`
	CanonicalHash      string                                       `json:"canonical_hash"`
	DraftRevision      int64                                        `json:"draft_revision,string"`
	CurrentDefinition  *TicketDefinitionRef                         `json:"current_definition_ref"`
	Changed            bool                                         `json:"changed"`
	ReferenceRevisions map[string]TicketDefinitionReferenceRevision `json:"reference_revisions"`
	Warnings           []TicketDefinitionWarning                    `json:"warnings"`
}

type TicketDefinitionPublicationReceipt struct {
	OperationID   string              `json:"operation_id"`
	Changed       bool                `json:"changed"`
	DefinitionRef TicketDefinitionRef `json:"definition_ref"`
	DraftRevision int64               `json:"draft_revision,string"`
}

// PublishedTicketForm contains only the immutable schema needed to create a ticket.
type PublishedTicketForm struct {
	Type          PublishedTicketFormType    `json:"type"`
	Fields        []PublishedTicketFormField `json:"fields"`
	DefinitionRef TicketDefinitionRef        `json:"definition_ref"`
}

type PublishedTicketFormType struct {
	Code            string          `json:"code"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	States          json.RawMessage `json:"states"`
	Transitions     json.RawMessage `json:"transitions"`
	DefaultSLAHours int             `json:"default_sla_hours"`
	HasCustomFields bool            `json:"has_custom_fields"`
	IsActive        bool            `json:"is_active"`
}

type PublishedTicketFormField struct {
	FieldID       int64                        `json:"field_id,string"`
	TypeCode      string                       `json:"type_code"`
	FieldKey      string                       `json:"field_key"`
	FieldLabel    string                       `json:"field_label"`
	FieldType     string                       `json:"field_type"`
	FieldOptions  json.RawMessage              `json:"field_options,omitempty"`
	FieldMeta     PublishedTicketFormFieldMeta `json:"field_meta"`
	Required      bool                         `json:"required"`
	ValidateRegex string                       `json:"validate_regex,omitempty"`
	SortOrder     int                          `json:"sort_order"`
}

// Field metadata sent to the form renderer. Server-side fetch source, action
// parameters and any unrecognized metadata stay on the server.
type PublishedTicketFormFieldMeta struct {
	ColSpan     *int   `json:"col_span,omitempty"`
	Group       string `json:"group,omitempty"`
	VisibleWhen string `json:"visible_when,omitempty"`
	Formula     string `json:"formula,omitempty"`
}
