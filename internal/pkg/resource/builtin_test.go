package resource

import (
	"context"
	"errors"
	"testing"
)

func TestBuiltinGetFilterShapes(t *testing.T) {
	ctx := context.Background()

	t.Run("org-member predicate", func(t *testing.T) {
		f, err := Builtin("task", OrgMember("jobs")).GetFilter(ctx, 42, "list")
		if err != nil {
			t.Fatal(err)
		}
		if f.Unscoped {
			t.Fatal("org-member filter must not be Unscoped (IW4 sentinel contract)")
		}
		if f.Where == "" || len(f.Args) != 1 || f.Args[0].(int64) != 42 {
			t.Fatalf("bad filter: %+v", f)
		}
	})
	t.Run("owner-only predicate", func(t *testing.T) {
		f, err := Builtin("profile", OwnerOnly("user_profiles")).GetFilter(ctx, 7, "list")
		if err != nil {
			t.Fatal(err)
		}
		if f.Unscoped || f.Where != "created_by = $1" || f.Args[0].(int64) != 7 {
			t.Fatalf("bad filter: %+v", f)
		}
	})
	t.Run("role-gated explicit unscoped (IW4)", func(t *testing.T) {
		f, err := Builtin("misc", RoleGated()).GetFilter(ctx, 1, "list")
		if err != nil {
			t.Fatal(err)
		}
		if !f.Unscoped || f.Where != "" || f.Args != nil {
			t.Fatalf("role-gated must be explicit Unscoped exemption, got %+v", f)
		}
	})
}

func TestBuiltinAuthorize(t *testing.T) {
	ctx := context.Background()

	t.Run("role-gated always true", func(t *testing.T) {
		ok, err := Builtin("misc", RoleGated()).Authorize(ctx, AuthorizeRequest{UserID: 1})
		if err != nil || !ok {
			t.Fatalf("want (true,nil), got (%v,%v)", ok, err)
		}
	})
	t.Run("admin bypass", func(t *testing.T) {
		ok, err := Builtin("task", OrgMember("jobs")).Authorize(ctx,
			AuthorizeRequest{UserID: 1, Roles: []string{"admin"}})
		if err != nil || !ok {
			t.Fatalf("want admin bypass, got (%v,%v)", ok, err)
		}
	})
	t.Run("owner-only row attribute", func(t *testing.T) {
		res := Builtin("profile", OwnerOnly("user_profiles"))
		ok, _ := res.Authorize(ctx, AuthorizeRequest{UserID: 7, Context: map[string]any{"created_by": int64(7)}})
		if !ok {
			t.Fatal("owner must pass")
		}
		ok, _ = res.Authorize(ctx, AuthorizeRequest{UserID: 8, Context: map[string]any{"created_by": int64(7)}})
		if ok {
			t.Fatal("non-owner must be denied")
		}
	})
	t.Run("org-member membership check", func(t *testing.T) {
		res := Builtin("task", OrgMember("jobs"))
		res.Membership = func(_ context.Context, userID, orgID int64) (bool, error) {
			return userID == 7 && orgID == 100, nil
		}
		ok, err := res.Authorize(ctx, AuthorizeRequest{UserID: 7, Context: map[string]any{"org_id": int64(100)}})
		if err != nil || !ok {
			t.Fatalf("member must pass, got (%v,%v)", ok, err)
		}
		ok, _ = res.Authorize(ctx, AuthorizeRequest{UserID: 8, Context: map[string]any{"org_id": int64(100)}})
		if ok {
			t.Fatal("non-member must be denied")
		}
	})
	t.Run("org-member missing org_id fails closed", func(t *testing.T) {
		res := Builtin("task", OrgMember("jobs"))
		res.Membership = func(context.Context, int64, int64) (bool, error) { return true, nil }
		if _, err := res.Authorize(ctx, AuthorizeRequest{UserID: 7}); err == nil {
			t.Fatal("missing org_id must error (fail-closed), not silently allow")
		}
	})
	t.Run("org-member missing membership dependency fails closed", func(t *testing.T) {
		res := Builtin("task", OrgMember("jobs"))
		if _, err := res.Authorize(ctx, AuthorizeRequest{UserID: 7, Context: map[string]any{"org_id": int64(100)}}); err == nil {
			t.Fatal("missing Membership must error (fail-closed)")
		}
	})
	t.Run("membership db error propagates", func(t *testing.T) {
		res := Builtin("task", OrgMember("jobs"))
		boom := errors.New("db down")
		res.Membership = func(context.Context, int64, int64) (bool, error) { return false, boom }
		if _, err := res.Authorize(ctx, AuthorizeRequest{UserID: 7, Context: map[string]any{"org_id": int64(100)}}); !errors.Is(err, boom) {
			t.Fatalf("db error must propagate, got %v", err)
		}
	})
}

func TestBuiltinRequireSchema(t *testing.T) {
	t.Run("org-member requires table and org_id column", func(t *testing.T) {
		res := Builtin("task", OrgMember("jobs"))
		if err := res.RequireSchema(func(table, column string) bool { return column == "org_id" }); err != nil {
			t.Fatalf("satisfied contract must pass, got %v", err)
		}
		if err := res.RequireSchema(func(table, column string) bool { return false }); err == nil {
			t.Fatal("missing column must fail fast")
		}
	})
	t.Run("missing table name errors", func(t *testing.T) {
		if err := Builtin("bad", BuiltinPolicy{kind: kindOrgMember}).RequireSchema(func(string, string) bool { return true }); err == nil {
			t.Fatal("org-member without table must error")
		}
	})
	t.Run("role-gated has no contract", func(t *testing.T) {
		if err := Builtin("misc", RoleGated()).RequireSchema(func(string, string) bool { return false }); err != nil {
			t.Fatalf("role-gated must always pass, got %v", err)
		}
	})
}

func TestBuiltinRegistryRoundtrip(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Builtin("task", OrgMember("jobs")))
	got, ok := reg.Get("task")
	if !ok || got.Code() != "task" {
		t.Fatal("builtin resource not registered")
	}
	f, err := reg.GetFilter(context.Background(), "task", 1, "list")
	if err != nil || f.Where == "" {
		t.Fatalf("registry GetFilter broken: (%+v, %v)", f, err)
	}
}
