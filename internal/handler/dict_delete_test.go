package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// 无效条件必须在调用 service 前拒绝，不能被忽略后退化成旧删除。
func TestDictDeleteRejectsInvalidConditions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct{ name, path, body string }{
		{"type_id_only", "/dicts/delete", `{"code":"synthetic","id":"1"}`},
		{"type_version_only", "/dicts/delete", `{"code":"synthetic","version":1}`},
		{"type_null_pair", "/dicts/delete", `{"code":"synthetic","id":null,"version":null}`},
		{"type_numeric_id", "/dicts/delete", `{"code":"synthetic","id":1,"version":1}`},
		{"type_zero_id", "/dicts/delete", `{"code":"synthetic","id":"0","version":1}`},
		{"type_negative_id", "/dicts/delete", `{"code":"synthetic","id":"-1","version":1}`},
		{"type_overflow_id", "/dicts/delete", `{"code":"synthetic","id":"9223372036854775808","version":1}`},
		{"type_long_id", "/dicts/delete", `{"code":"synthetic","id":"` + strings.Repeat("1", 1000) + `","version":1}`},
		{"type_long_code", "/dicts/delete", `{"code":"` + strings.Repeat("c", 65) + `"}`},
		{"type_string_version", "/dicts/delete", `{"code":"synthetic","id":"1","version":"1"}`},
		{"type_null_version", "/dicts/delete", `{"code":"synthetic","id":"1","version":null}`},
		{"type_zero_version", "/dicts/delete", `{"code":"synthetic","id":"1","version":0}`},
		{"type_negative_version", "/dicts/delete", `{"code":"synthetic","id":"1","version":-1}`},
		{"type_fraction_version", "/dicts/delete", `{"code":"synthetic","id":"1","version":1.5}`},
		{"type_overflow_version", "/dicts/delete", `{"code":"synthetic","id":"1","version":9223372036854775808}`},
		{"type_long_version", "/dicts/delete", `{"code":"synthetic","id":"1","version":` + strings.Repeat("9", 1000) + `}`},
		{"item_missing_id", "/dict-items/delete", `{}`},
		{"item_numeric_id", "/dict-items/delete", `{"id":1}`},
		{"item_null_id", "/dict-items/delete", `{"id":null}`},
		{"item_zero_id", "/dict-items/delete", `{"id":"0"}`},
		{"item_negative_id", "/dict-items/delete", `{"id":"-1"}`},
		{"item_overflow_id", "/dict-items/delete", `{"id":"9223372036854775808"}`},
		{"item_long_id", "/dict-items/delete", `{"id":"` + strings.Repeat("1", 1000) + `"}`},
		{"item_null_version", "/dict-items/delete", `{"id":"1","version":null}`},
		{"item_string_version", "/dict-items/delete", `{"id":"1","version":"1"}`},
		{"item_zero_version", "/dict-items/delete", `{"id":"1","version":0}`},
		{"item_negative_version", "/dict-items/delete", `{"id":"1","version":-1}`},
		{"item_fraction_version", "/dict-items/delete", `{"id":"1","version":1.5}`},
		{"item_overflow_version", "/dict-items/delete", `{"id":"1","version":9223372036854775808}`},
		{"item_long_version", "/dict-items/delete", `{"id":"1","version":` + strings.Repeat("9", 1000) + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(gin.RecoveryWithWriter(io.Discard))
			h := NewDictHandler(nil)
			r.POST("/dicts/delete", h.DeleteType)
			r.POST("/dict-items/delete", h.DeleteItem)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}

// 只替换数据库边界；真实 handler/service/repository 负责校验、SQL 和错误映射。
// 这些断言不代替真实 PostgreSQL MVCC/级联测试。
func TestDictDeleteContract(t *testing.T) {
	for _, tc := range []struct {
		name, path, body                     string
		args                                 []any
		cas, exists, deleted, inUse, dbError bool
		wantStatus, wantCode                 int
	}{
		{"type_cas", "/dicts/delete", `{"code":"synthetic","id":"9007199254740993","version":7}`, []any{"synthetic", int64(9007199254740993), int64(7)}, true, true, true, false, false, 200, 0},
		{"type_replaced_id", "/dicts/delete", `{"code":"synthetic","id":"11","version":7}`, []any{"synthetic", int64(11), int64(7)}, true, true, false, false, false, 409, errcode.ErrConflict.Code},
		{"type_stale_version", "/dicts/delete", `{"code":"synthetic","id":"12","version":6}`, []any{"synthetic", int64(12), int64(6)}, true, true, false, false, false, 409, errcode.ErrConflict.Code},
		{"type_cas_missing", "/dicts/delete", `{"code":"synthetic","id":"12","version":7}`, []any{"synthetic", int64(12), int64(7)}, true, false, false, false, false, 404, errcode.ErrDictTypeNotFound.Code},
		{"type_legacy", "/dicts/delete", `{"code":"synthetic"}`, []any{"synthetic"}, false, true, true, false, false, 200, 0},
		{"type_legacy_missing", "/dicts/delete", `{"code":"synthetic"}`, []any{"synthetic"}, false, false, false, false, false, 404, errcode.ErrDictTypeNotFound.Code},
		{"type_guard_cas", "/dicts/delete", `{"code":"synthetic","id":"12","version":7}`, []any{"synthetic", int64(12), int64(7)}, true, true, false, true, false, 400, errcode.ErrDictTypeInUse.Code},
		{"type_guard_legacy", "/dicts/delete", `{"code":"synthetic"}`, []any{"synthetic"}, false, true, false, true, false, 400, errcode.ErrDictTypeInUse.Code},
		{"type_db_error", "/dicts/delete", `{"code":"synthetic","id":"12","version":7}`, []any{"synthetic", int64(12), int64(7)}, true, true, false, false, true, 500, errcode.ErrInternal.Code},
		{"type_int64_max", "/dicts/delete", `{"code":"` + strings.Repeat("c", 64) + `","id":"9223372036854775807","version":9223372036854775807}`, []any{strings.Repeat("c", 64), int64(9223372036854775807), int64(9223372036854775807)}, true, true, true, false, false, 200, 0},
		{"item_cas", "/dict-items/delete", `{"id":"9007199254740993","version":7}`, []any{int64(9007199254740993), int64(7)}, true, true, true, false, false, 200, 0},
		{"item_stale_version", "/dict-items/delete", `{"id":"12","version":6}`, []any{int64(12), int64(6)}, true, true, false, false, false, 409, errcode.ErrConflict.Code},
		{"item_cas_missing", "/dict-items/delete", `{"id":"12","version":7}`, []any{int64(12), int64(7)}, true, false, false, false, false, 404, errcode.ErrDictItemNotFound.Code},
		{"item_legacy", "/dict-items/delete", `{"id":"12"}`, []any{int64(12)}, false, true, true, false, false, 200, 0},
		{"item_legacy_missing", "/dict-items/delete", `{"id":"12"}`, []any{int64(12)}, false, false, false, false, false, 404, errcode.ErrDictItemNotFound.Code},
		{"item_db_error", "/dict-items/delete", `{"id":"12","version":7}`, []any{int64(12), int64(7)}, true, true, false, false, true, 500, errcode.ErrInternal.Code},
		{"item_int64_max", "/dict-items/delete", `{"id":"9223372036854775807","version":9223372036854775807}`, []any{int64(9223372036854775807), int64(9223372036854775807)}, true, true, true, false, false, 200, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &dictDeleteDB{t: t, typeDelete: tc.path == "/dicts/delete", cas: tc.cas, args: tc.args, exists: tc.exists, deleted: tc.deleted, inUse: tc.inUse, dbError: tc.dbError}
			h := NewDictHandler(service.NewDictService(repository.NewDictRepo(db), slog.New(slog.NewTextHandler(io.Discard, nil))))
			r := gin.New()
			r.POST("/dicts/delete", h.DeleteType)
			r.POST("/dict-items/delete", h.DeleteItem)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			var body struct {
				Code int `json:"code"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, tc.wantCode, body.Code)
			if db.typeDelete {
				require.True(t, db.guardChecked)
			}
			if tc.inUse {
				require.Zero(t, db.deletes)
			} else {
				require.Equal(t, 1, db.deletes)
			}
		})
	}
}

type dictDeleteDB struct {
	t                                                              *testing.T
	typeDelete, cas, exists, deleted, inUse, dbError, guardChecked bool
	args                                                           []any
	deletes                                                        int
}

type dictDeleteRow struct {
	values []bool
	err    error
}

func (r dictDeleteRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, v := range r.values {
		*dest[i].(*bool) = v
	}
	return nil
}

func (db *dictDeleteDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "jsonb_array_elements_text") {
		require.True(db.t, db.typeDelete)
		require.Contains(db.t, sql, "WHERE e = 'dict:' || $1")
		require.Equal(db.t, db.args[:1], args)
		db.guardChecked = true
		return dictDeleteRow{values: []bool{db.inUse}}
	}
	if db.typeDelete {
		require.True(db.t, db.guardChecked)
	}
	require.True(db.t, db.cas, "legacy request must use its existing DELETE")
	require.Contains(db.t, sql, "FOR UPDATE")
	require.Contains(db.t, sql, "USING target")
	if db.typeDelete {
		require.Contains(db.t, sql, "SELECT id FROM dict_types WHERE code = $1 FOR UPDATE")
		require.Contains(db.t, sql, "DELETE FROM dict_types d")
		require.Contains(db.t, sql, "d.code = $1")
		require.Contains(db.t, sql, "d.id = $2")
		require.Contains(db.t, sql, "d.version = $3")
	} else {
		require.Contains(db.t, sql, "SELECT id FROM dict_items WHERE id = $1 FOR UPDATE")
		require.Contains(db.t, sql, "DELETE FROM dict_items d")
		require.Contains(db.t, sql, "d.id = $1")
		require.Contains(db.t, sql, "d.version = $2")
	}
	require.Contains(db.t, sql, "d.id = target.id")
	require.Contains(db.t, sql, "SELECT EXISTS (SELECT 1 FROM target), EXISTS (SELECT 1 FROM deleted)")
	require.Equal(db.t, db.args, args)
	db.deletes++
	if db.dbError {
		return dictDeleteRow{err: errors.New("synthetic DB failure")}
	}
	return dictDeleteRow{values: []bool{db.exists, db.deleted}}
}

func (db *dictDeleteDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	require.False(db.t, db.cas, "CAS must never fall back to unconditional DELETE")
	if db.typeDelete {
		require.True(db.t, db.guardChecked)
		require.Equal(db.t, "DELETE FROM dict_types WHERE code = $1", sql)
	} else {
		require.Equal(db.t, "DELETE FROM dict_items WHERE id = $1", sql)
	}
	require.Equal(db.t, db.args, args)
	db.deletes++
	if db.deleted {
		return pgconn.NewCommandTag("DELETE 1"), nil
	}
	return pgconn.NewCommandTag("DELETE 0"), nil
}

func (db *dictDeleteDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	db.t.Fatal("unexpected list query during deletion")
	return nil, nil
}
