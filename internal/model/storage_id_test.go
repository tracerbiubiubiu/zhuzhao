package model

import (
	"encoding/json"
	"testing"
)

func TestStorageModel_IDWireContract(t *testing.T) {
	const id int64 = 1<<53 + 1
	for name, value := range map[string]any{
		"file metadata":       FileObject{ID: id},
		"attachment view":     TicketAttachmentView{ID: id},
		"attachment relation": TicketAttachment{ID: id},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if string(fields["id"]) != `"9007199254740993"` {
				t.Fatalf("ID must be an exact decimal JSON string, got %s", fields["id"])
			}
		})
	}
}

func TestStorageInput_TicketIDWireContract(t *testing.T) {
	var confirm ConfirmFileInput
	if err := json.Unmarshal([]byte(`{"object_key":"tickets/synthetic.png","ticket_id":"9007199254740993"}`), &confirm); err != nil {
		t.Fatal(err)
	}
	if confirm.TicketID == nil || *confirm.TicketID != 1<<53+1 {
		t.Fatalf("input string ID lost precision: %+v", confirm.TicketID)
	}
	if err := json.Unmarshal([]byte(`{"object_key":"tickets/synthetic.png","ticket_id":9007199254740993}`), &confirm); err == nil {
		t.Fatal("the existing string-only input contract must remain unchanged")
	}
}
