//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// ---------- 模板预填（P1-4：default_fields 预填空缺字段 + default_priority 缺省） ----------

// seedTemplate 插入测试模板（幂等：partial unique index ON CONFLICT 谓词须完全匹配）
func seedTemplate(t *testing.T, oid, uid int64) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO ticket_templates (code, name, type_code, default_priority, default_fields, org_id, org_path, created_by)
		VALUES ('tpl_prefill', '预填模板', 'incident', 2,
			'{"description":"模板预填描述","custom_data":{"source":"template","level":2}}'::jsonb,
			$1, (SELECT path FROM organizations WHERE id = $1), $2)
		ON CONFLICT (code) WHERE deleted_at IS NULL
			DO UPDATE SET default_fields = EXCLUDED.default_fields, default_priority = EXCLUDED.default_priority`,
		oid, uid)
	require.NoError(t, err)
	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), `DELETE FROM ticket_templates WHERE code = 'tpl_prefill'`); err != nil {
			t.Logf("cleanup: delete tpl_prefill: %v", err)
		}
	})
}

// TestCreate_TemplatePrefill 命中模板时空缺字段被预填：description/custom_data 来自
// default_fields，priority 取 default_priority（请求未显式指定时）
func TestCreate_TemplatePrefill(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	oid := childOrgID(t, "tpl1")
	roles[aid] = []string{"operator"}
	seedTemplate(t, oid, aid)
	ctx := context.Background()

	tk, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode:     "incident",
		Title:        "模板预填-空缺字段",
		OrgID:        oid,
		TemplateCode: "tpl_prefill",
	}, aid)
	require.NoError(t, err)
	assert.Equal(t, "模板预填描述", tk.Description, "description 空缺时应被模板预填")
	assert.Equal(t, 2, tk.Priority, "priority 未指定时应取模板 default_priority=2")
	require.NotNil(t, tk.CustomData, "custom_data 空缺时应被模板预填")
	assert.JSONEq(t, `{"source":"template","level":2}`, string(tk.CustomData))
}

// TestCreate_TemplateExplicitWins 请求显式值优先：description/priority 均保留用户输入，
// 模板不覆盖（09-ticket「预填」语义，非「覆盖」）
func TestCreate_TemplateExplicitWins(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	oid := childOrgID(t, "tpl2")
	roles[aid] = []string{"operator"}
	seedTemplate(t, oid, aid)
	ctx := context.Background()

	tk, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode:     "incident",
		Title:        "模板预填-显式优先",
		Description:  "用户显式描述",
		Priority:     1,
		OrgID:        oid,
		TemplateCode: "tpl_prefill",
		CustomData:   json.RawMessage(`{"source":"user"}`),
	}, aid)
	require.NoError(t, err)
	assert.Equal(t, "用户显式描述", tk.Description)
	assert.Equal(t, 1, tk.Priority, "显式 priority=1 不得被模板 default_priority=2 覆盖")
	assert.JSONEq(t, `{"source":"user"}`, string(tk.CustomData), "显式 custom_data 不与模板深合并")
}

// TestCreate_TemplateTypeMismatch 模板类型与请求 type_code 不一致 → 400（ErrInvalidParams）
func TestCreate_TemplateTypeMismatch(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	oid := childOrgID(t, "tpl3")
	roles[aid] = []string{"operator"}
	seedTemplate(t, oid, aid)
	ctx := context.Background()

	_, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode:     "request", // 模板属于 incident
		Title:        "类型不匹配",
		OrgID:        oid,
		TemplateCode: "tpl_prefill",
	}, aid)
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)
}

// TestCreate_TemplateNotExist 模板不存在是可选参数：静默跳过，正常建单、走类型缺省优先级
func TestCreate_TemplateNotExist(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	oid := childOrgID(t, "tpl4")
	roles[aid] = []string{"operator"}
	ctx := context.Background()

	tk, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode:     "incident",
		Title:        "模板不存在",
		OrgID:        oid,
		TemplateCode: "tpl_nope_never_exist",
	}, aid)
	require.NoError(t, err)
	assert.Empty(t, tk.Description)
	assert.Equal(t, defaultTicketPriority, tk.Priority, "无模板时回落类型缺省优先级 3")
}

// TestCreate_InactiveTypeRejected 停用类型建单 → 90003（对客户端视同类型不存在）
func TestCreate_InactiveTypeRejected(t *testing.T) {
	svc, aid, _, _, roles := setupTicket2a(t)
	oid := childOrgID(t, "tpl5")
	roles[aid] = []string{"operator"}
	ctx := context.Background()

	_, err := testPool.Exec(ctx, `
		INSERT INTO ticket_types (code, name, description, is_active)
		VALUES ('inactive_tpl_type', '停用类型（测试）', 'x', false)
		ON CONFLICT DO NOTHING`)
	require.NoError(t, err)

	_, err = svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: "inactive_tpl_type",
		Title:    "停用类型建单",
		OrgID:    oid,
	}, aid)
	requireErrCode(t, err, errcode.ErrTicketTypeNotFound.Code)
}
