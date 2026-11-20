//go:build integration

package repository_test

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"testing"
)

func TestConfigStableTargetRecreatedType(t *testing.T) {
	ctx := context.Background()
	r := repository.NewTicketRepo(testPool)
	code := insertCfgType(t, "original")
	old, err := r.GetTicketType(ctx, code)
	require.NoError(t, err)
	require.NoError(t, r.DeleteTicketType(ctx, code))
	require.NoError(t, r.CreateTicketType(ctx, &model.TicketType{Code: code, Name: "replacement", States: old.States, Transitions: old.Transitions, IsActive: true}))
	current, err := r.GetTicketType(ctx, code)
	require.NoError(t, err)
	require.NotEqual(t, old.ID, current.ID)
	version := int(old.Version)
	name := "old draft"
	_, err = r.UpdateTicketType(ctx, code, &name, nil, nil, nil, nil, nil, &version, old.ID)
	require.ErrorIs(t, err, errcode.ErrConcurrentModification)
	require.ErrorIs(t, r.ReplaceTypeFields(ctx, code, []model.TicketTypeField{{FieldKey: "normal", FieldLabel: "Normal", FieldType: "input"}}, &version, old.ID), errcode.ErrConcurrentModification)
	require.ErrorIs(t, r.DeleteTicketType(ctx, code, repository.ConfigTarget{ID: &old.ID, Version: &version}), errcode.ErrConcurrentModification)
	got, err := r.GetTicketType(ctx, code)
	require.NoError(t, err)
	require.Equal(t, "replacement", got.Name)
	require.Equal(t, current.Version, got.Version)
	fields, err := r.ListTicketTypeFieldsAll(ctx, code)
	require.NoError(t, err)
	require.Empty(t, fields)
	_, err = r.UpdateTicketType(ctx, code, &name, nil, nil, nil, nil, nil, &version, current.ID)
	require.NoError(t, err)
}
func TestConfigStableTargetRecreatedTemplate(t *testing.T) {
	ctx := context.Background()
	r := repository.NewTicketRepo(testPool)
	_, code := setupCfgTemplate(t)
	old, err := r.GetTicketTemplate(ctx, code)
	require.NoError(t, err)
	require.NoError(t, r.DeleteTicketTemplate(ctx, code))
	replacement := *old
	replacement.Name = "replacement"
	require.NoError(t, r.CreateTicketTemplate(ctx, &replacement))
	version := int(old.Version)
	name := "old draft"
	_, err = r.UpdateTicketTemplate(ctx, code, &name, nil, nil, nil, &version, old.ID)
	require.ErrorIs(t, err, errcode.ErrConcurrentModification)
	require.ErrorIs(t, r.DeleteTicketTemplate(ctx, code, repository.ConfigTarget{ID: &old.ID, Version: &version}), errcode.ErrConcurrentModification)
	got, err := r.GetTicketTemplate(ctx, code)
	require.NoError(t, err)
	require.Equal(t, "replacement", got.Name)
	require.NoError(t, r.DeleteTicketTemplate(ctx, code, repository.ConfigTarget{ID: &got.ID, Version: &version}))
}

func TestConfigStableTargetDictionaryParentAndRemark(t *testing.T) {
	ctx := context.Background()
	r := repository.NewDictRepo(testPool)
	old := &model.DictType{Code: cfgUnique("dict_parent"), Name: "original", Enabled: true, Remark: "original remark"}
	require.NoError(t, r.CreateType(ctx, old))
	require.NoError(t, r.DeleteTypeByCode(ctx, old.Code))
	current := &model.DictType{Code: old.Code, Name: "replacement", Enabled: true, Remark: "new parent remark"}
	require.NoError(t, r.CreateType(ctx, current))
	t.Cleanup(func() { _ = r.DeleteTypeByCode(ctx, current.Code) })
	item := &model.DictItem{TypeCode: old.Code, Code: "normal", Label: "Normal", Enabled: true, Remark: "item remark"}
	require.ErrorIs(t, r.CreateItem(ctx, item, old.ID), repository.ErrDictDeleteConflict)
	items, _, err := r.ListItems(ctx, 1, 100, current.Code)
	require.NoError(t, err)
	require.Empty(t, items)
	require.NoError(t, r.CreateItem(ctx, item, current.ID))
	var omitted *string
	require.NoError(t, r.UpdateType(ctx, current, omitted))
	require.NoError(t, r.UpdateItem(ctx, item, omitted))
	var remark string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT remark FROM dict_types WHERE id=$1`, current.ID).Scan(&remark))
	require.Equal(t, "new parent remark", remark)
	require.NoError(t, testPool.QueryRow(ctx, `SELECT remark FROM dict_items WHERE id=$1`, item.ID).Scan(&remark))
	require.Equal(t, "item remark", remark)
	current.Version++
	item.Version++
	clear := ""
	require.NoError(t, r.UpdateType(ctx, current, &clear))
	require.NoError(t, r.UpdateItem(ctx, item, &clear))
	require.NoError(t, testPool.QueryRow(ctx, `SELECT remark FROM dict_types WHERE id=$1`, current.ID).Scan(&remark))
	require.Empty(t, remark)
	require.NoError(t, testPool.QueryRow(ctx, `SELECT remark FROM dict_items WHERE id=$1`, item.ID).Scan(&remark))
	require.Empty(t, remark)
	require.NoError(t, r.CreateItem(ctx, &model.DictItem{TypeCode: old.Code, Code: "legacy", Label: "Legacy", Enabled: true}))
}

func TestRoleMenuSnapshotConcurrentAndPolicyAtomic(t *testing.T) {
	ctx := context.Background()
	r := repository.NewRoleRepo(testPool)
	role := &model.Role{ID: insertRole(t, cfgUnique("menu_snapshot"), 80, 1, false)}
	require.NoError(t, testPool.QueryRow(ctx, `SELECT code FROM roles WHERE id=$1`, role.ID).Scan(&role.Code))
	var menus [2]int64
	for i := range menus {
		require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO menus(code,name,menu_type) VALUES($1,'Normal menu',3) RETURNING id`, cfgUnique("snapshot_menu")).Scan(&menus[i]))
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1`, "role::"+role.Code)
		_, _ = testPool.Exec(ctx, `DELETE FROM role_menus WHERE role_id=$1`, role.ID)
		_, _ = testPool.Exec(ctx, `DELETE FROM roles WHERE id=$1`, role.ID)
		_, _ = testPool.Exec(ctx, `DELETE FROM menus WHERE id=ANY($1)`, menus[:])
	})
	paths := []string{"/api/v1/normal_a", "/api/v1/normal_b"}
	errs := runConcurrently(2, func(i int) error {
		return r.AssignMenus(ctx, role, []int64{menus[i]}, []*model.MenuAPI{{APIPath: paths[i], APIMethod: "GET"}}, []int64{})
	})
	assertExactlyOneConflict(t, errs)
	ids, err := r.ListMenuIDsByRoleID(ctx, role.ID)
	require.NoError(t, err)
	require.Len(t, ids, 1)
	winner := 0
	if ids[0] == menus[1] {
		winner = 1
	}
	require.Equal(t, menus[winner], ids[0])
	policies, err := r.ListCasbinPoliciesByRoleCode(ctx, role.Code)
	require.NoError(t, err)
	require.Equal(t, [][3]string{{"role::" + role.Code, paths[winner], "GET"}}, policies, "conflicting replacement must not write bindings or policies")
}
