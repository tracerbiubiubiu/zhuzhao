package handler

import (
	"context"
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
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// Exercise the real HTTP -> service -> repository path; only the database is a fixture.
func TestDictUpdateRemarkPresence(t *testing.T) {
	for _, route := range []string{"/dicts/update", "/dict-items/update"} {
		for _, tc := range []struct{ name, suffix, want string }{
			{"omitted", "", "original remark"}, {"clear", `,"remark":""`, ""}, {"replace", `,"remark":"new remark"`, "new remark"},
		} {
			t.Run(route+tc.name, func(t *testing.T) {
				db := &dictPatchDB{remark: "original remark"}
				h := NewDictHandler(service.NewDictService(repository.NewDictRepo(db), slog.New(slog.NewTextHandler(io.Discard, nil))))
				r := gin.New()
				r.POST("/dicts/update", h.UpdateType)
				r.POST("/dict-items/update", h.UpdateItem)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"id":"12","name":"type","label":"item","version":1`+tc.suffix+`}`)))
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, tc.want, db.remark)
			})
		}
	}
}

type dictPatchDB struct{ remark string }

func (db *dictPatchDB) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	i := 3
	if strings.Contains(q, "dict_items") {
		i = 4
	}
	switch v := args[i].(type) {
	case string:
		db.remark = v
	case *string:
		if v != nil {
			db.remark = *v
		}
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (*dictPatchDB) QueryRow(context.Context, string, ...any) pgx.Row { panic("unexpected query") }
func (*dictPatchDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected query")
}
func (*dictPatchDB) Begin(context.Context) (pgx.Tx, error) { panic("unexpected begin") }

func TestDictCreateItemParentIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, suffix string
		status       int
	}{
		{"current", `,"type_id":"9007199254740993"`, 200},
		{"recreated", `,"type_id":"12"`, 409},
		{"legacy", "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &dictParentDB{t: t}
			h := NewDictHandler(service.NewDictService(repository.NewDictRepo(db), slog.New(slog.NewTextHandler(io.Discard, nil))))
			r := gin.New()
			r.POST("/dict-items", h.CreateItem)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("POST", "/dict-items", strings.NewReader(`{"type_code":"normal","code":"item","label":"Normal"`+tc.suffix+`}`)))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.name != "legacy" {
				require.Len(t, db.args, 7)
			} else {
				require.Len(t, db.args, 6)
			}
		})
	}
}

type dictParentDB struct {
	dictPatchDB
	t    *testing.T
	args []any
}

func (db *dictParentDB) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	db.args = args
	if len(args) == 7 {
		require.Contains(db.t, q, "FOR KEY SHARE")
		if args[6].(int64) != int64(9007199254740993) {
			return dictParentRow{err: pgx.ErrNoRows}
		}
	}
	return dictParentRow{}
}

type dictParentRow struct{ err error }

func (r dictParentRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*int64) = 41
	*dest[1].(*int64) = 1
	return nil
}
