package jobs

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRegistryRegisterGet(t *testing.T) {
	r := NewRegistry()
	h := HandlerFunc(func(ctx context.Context, params json.RawMessage) error { return nil })
	r.Register("audit_archive", h)

	got, ok := r.Get("audit_archive")
	if !ok || got == nil {
		t.Fatal("registered handler not found")
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("unknown action must not be found (P6: 404 快速失败)")
	}
	if acts := r.Actions(); len(acts) != 1 || acts[0] != "audit_archive" {
		t.Fatalf("actions = %v", acts)
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	r := NewRegistry()
	h := HandlerFunc(func(context.Context, json.RawMessage) error { return nil })
	r.Register("x", h)
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate register must panic (装配期 fail-fast)")
		}
	}()
	r.Register("x", h)
}
