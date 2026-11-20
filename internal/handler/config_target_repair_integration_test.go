//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// Real JSON binding -> service -> SQL; synthetic database fixture only.
func TestConfigStableTargetHTTP(t *testing.T) {
	ctx := context.Background()
	tr := repository.NewTicketRepo(testPool)
	dr := repository.NewDictRepo(testPool)
	ts := ticketsvc.NewTicketService(testPool, tr, nil, resource.NewRegistry(), nil, nil, nil, nil, nil)
	th := handler.NewTicketHandler(ts, nil, nil)
	dh := handler.NewDictHandler(service.NewDictService(dr, slog.New(slog.NewTextHandler(io.Discard, nil))))
	r := gin.New()
	r.POST("/types/update", th.UpdateTicketType)
	r.POST("/types/fields", th.ReplaceTicketTypeFields)
	r.POST("/types/delete", th.DeleteTicketType)
	r.POST("/templates/update", th.UpdateTicketTemplate)
	r.POST("/templates/delete", th.DeleteTicketTemplate)
	r.POST("/dict-items", dh.CreateItem)
	r.POST("/dict-items/update", dh.UpdateItem)
	r.POST("/dicts/update", dh.UpdateType)
	post := func(path, body string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
		require.Equal(t, want, w.Code, w.Body.String())
	}
	code := fmt.Sprintf("http_cfg_%d", time.Now().UnixNano())
	old, err := ts.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "old"})
	require.NoError(t, err)
	require.NoError(t, tr.DeleteTicketType(ctx, code))
	current, err := ts.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "new"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.DeleteTicketType(ctx, code) })
	for _, path := range []string{"/types/update", "/types/fields", "/types/delete"} {
		post(path, fmt.Sprintf(`{"code":%q,"id":"%d","version":1,"name":"old draft","states":["open","assigned","closed"],"transitions":{"open":["assigned"],"assigned":["closed"],"closed":["open"]},"fields":[]}`, code, old.ID), 409)
	}
	got, err := tr.GetTicketType(ctx, code)
	require.NoError(t, err)
	require.Equal(t, current.ID, got.ID)
	require.Equal(t, "new", got.Name)
	post("/types/update", fmt.Sprintf(`{"code":%q,"id":"%d","version":1,"name":"new draft"}`, code, current.ID), 200)
	orgCode := code + "_org"
	var orgID int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path) VALUES($1,$2,$3::ltree) RETURNING id`, orgCode, orgCode, orgCode).Scan(&orgID))
	t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, orgID) })
	template := &model.TicketTemplate{Code: code, Name: "old", TypeCode: code, OrgID: orgID, OrgPath: orgCode, DefaultPriority: 3, DefaultFields: json.RawMessage(`{}`)}
	require.NoError(t, tr.CreateTicketTemplate(ctx, template))
	oldTemplateID := template.ID
	require.NoError(t, tr.DeleteTicketTemplate(ctx, code))
	template.Name = "new"
	require.NoError(t, tr.CreateTicketTemplate(ctx, template))
	t.Cleanup(func() { _ = tr.DeleteTicketTemplate(ctx, code) })
	for _, path := range []string{"/templates/update", "/templates/delete"} {
		post(path, fmt.Sprintf(`{"code":%q,"id":"%d","version":1,"name":"old draft"}`, code, oldTemplateID), 409)
	}
	templateRead, err := tr.GetTicketTemplate(ctx, code)
	require.NoError(t, err)
	require.Equal(t, template.ID, templateRead.ID)
	require.Equal(t, "new", templateRead.Name)
	post("/templates/update", fmt.Sprintf(`{"code":%q,"id":"%d","version":1,"name":"new draft"}`, code, template.ID), 200)
	dict := &model.DictType{Code: code, Name: "old", Enabled: true}
	require.NoError(t, dr.CreateType(ctx, dict))
	oldID := dict.ID
	require.NoError(t, dr.DeleteTypeByCode(ctx, code))
	dict = &model.DictType{Code: code, Name: "new", Enabled: true, Remark: "parent remark"}
	require.NoError(t, dr.CreateType(ctx, dict))
	t.Cleanup(func() { _ = dr.DeleteTypeByCode(ctx, code) })
	post("/dict-items", fmt.Sprintf(`{"type_code":%q,"type_id":"%d","code":"normal","label":"Normal"}`, code, oldID), 409)
	post("/dict-items", fmt.Sprintf(`{"type_code":%q,"type_id":"%d","code":"normal","label":"Normal","remark":"item remark"}`, code, dict.ID), 200)
	items, _, err := dr.ListItems(ctx, 1, 20, code)
	require.NoError(t, err)
	require.Len(t, items, 1)
	for _, suffix := range []string{"", `,"remark":""`} {
		post("/dicts/update", fmt.Sprintf(`{"id":"%d","name":"new","version":%d%s}`, dict.ID, dict.Version, suffix), 200)
		post("/dict-items/update", fmt.Sprintf(`{"id":"%d","label":"Normal","version":%d%s}`, items[0].ID, items[0].Version, suffix), 200)
		expected := "parent remark"
		if suffix != "" {
			expected = ""
		}
		var remark string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT remark FROM dict_types WHERE id=$1`, dict.ID).Scan(&remark))
		require.Equal(t, expected, remark)
		read, _, err := dr.ListItems(ctx, 1, 20, code)
		require.NoError(t, err)
		if suffix == "" {
			require.Equal(t, "item remark", read[0].Remark)
		} else {
			require.Empty(t, read[0].Remark)
		}
		dict.Version++
		items[0].Version++
	}
}
