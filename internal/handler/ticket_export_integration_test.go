//go:build integration

package handler_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type exportAdminRoles struct{}

func (exportAdminRoles) GetRoleCodesByUserID(context.Context, int64) ([]string, error) {
	return []string{"admin"}, nil
}

type exportAudit struct {
	entries []middleware.AuditLogEntry
	cancel  context.CancelFunc
}

func (a *exportAudit) Insert(_ context.Context, entry middleware.AuditLogEntry) error {
	a.entries = append(a.entries, entry)
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

type exportBrokenWriter struct{ header http.Header }

func (w *exportBrokenWriter) Header() http.Header     { return w.header }
func (*exportBrokenWriter) WriteHeader(int)           {}
func (*exportBrokenWriter) Write([]byte) (int, error) { return 0, context.Canceled }

// TestTicketExportWorkbook exercises the real handler, service and PG row stream,
// then parses the emitted worksheet strictly (Excelize's reader tolerates a
// missing closing tag). A >16MiB sheet also exercises real temporary-file spill.
func TestTicketExportWorkbook(t *testing.T) {
	pool := testPool
	ctx := context.Background()
	suffix := fmt.Sprintf("export_%d", time.Now().UnixNano())
	var uid, oid int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO users(username,password,employee_no,status) VALUES($1,'hash',$1,1) RETURNING id`, suffix).Scan(&uid))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO organizations(code,name,path,status) VALUES($1::text,$1::text,$1::text::ltree,1) RETURNING id`, suffix).Scan(&oid))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tickets WHERE created_by=$1`, uid)
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id=$1`, oid)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, uid)
	})
	svc := ticketsvc.NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), resource.NewRegistry(), exportAdminRoles{}, nil, nil, nil, nil)
	for idx, tc := range []struct {
		name                    string
		rows                    int
		spill, broken, canceled bool
	}{
		{name: "empty", rows: 0}, {name: "small", rows: 1},
		{name: "spill", rows: 600, spill: true},
		{name: "spill_writer_failure", rows: 600, spill: true, broken: true},
		{name: "spill_request_canceled_before_write", rows: 600, spill: true, broken: true, canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keyword := fmt.Sprintf("%s_%d", suffix, idx)
			if tc.rows > 0 {
				size := 10
				if tc.spill {
					size = 32760
				}
				_, err := pool.Exec(ctx, `INSERT INTO tickets(type_code,title,description,status,created_by,org_id,org_path) SELECT 'incident',$1,repeat('x',$2),'closed',$3,$4,$5::ltree FROM generate_series(1,$6)`, keyword, size, uid, oid, suffix, tc.rows)
				require.NoError(t, err)
			}
			tempDir := t.TempDir()
			t.Setenv("TMPDIR", tempDir)
			rec := httptest.NewRecorder()
			var writer http.ResponseWriter = rec
			if tc.broken {
				writer = &exportBrokenWriter{header: http.Header{}}
			}
			c, _ := gin.CreateTestContext(writer)
			reqCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tickets/export?keyword="+keyword, nil).WithContext(reqCtx)
			c.Set("userID", uid)
			audit := &exportAudit{}
			if tc.canceled {
				audit.cancel = cancel
			}
			handler.NewTicketHandler(svc, audit, nil).Export(c)
			require.Len(t, audit.entries, 1)
			assert.Equal(t, fmt.Sprintf("rows=%d", tc.rows), audit.entries[0].ResponseSummary)
			entries, err := os.ReadDir(tempDir)
			require.NoError(t, err)
			assert.Empty(t, entries, "export must close Excelize's real spill files, including write failure/cancellation")
			if tc.broken {
				require.NotEmpty(t, c.Errors)
				assert.ErrorIs(t, c.Errors.Last().Err, context.Canceled)
				return
			}
			assert.Equal(t, http.StatusOK, rec.Code)
			if dir := os.Getenv("ZHUZHAO_EXPORT_EVIDENCE_DIR"); dir != "" {
				require.NoError(t, os.MkdirAll(dir, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, tc.name+".xlsx"), rec.Body.Bytes(), 0600))
			}
			zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
			require.NoError(t, err)
			found := false
			for _, f := range zr.File {
				if f.Name != "xl/worksheets/sheet1.xml" {
					continue
				}
				found = true
				if tc.spill {
					assert.Greater(t, f.UncompressedSize64, uint64(excelize.StreamChunkSize), "fixture must exercise actual >16MiB worksheet spill")
				}
				r, err := f.Open()
				require.NoError(t, err)
				decoder := xml.NewDecoder(r)
				for {
					_, err = decoder.Token()
					if err != nil {
						break
					}
				}
				assert.True(t, errors.Is(err, io.EOF), "worksheet must be complete XML: %v", err)
				require.NoError(t, r.Close())
			}
			require.True(t, found)
			book, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
			require.NoError(t, err)
			defer book.Close()
			rows, err := book.GetRows("Sheet1")
			require.NoError(t, err)
			require.Len(t, rows, tc.rows+1)
			assert.Equal(t, []string{"ID", "编号", "类型", "标题", "描述", "优先级", "状态", "创建人ID", "创建人", "处理人ID", "处理人", "组织路径", "创建时间", "更新时间", "进入状态时间"}, rows[0])
			if tc.rows > 0 {
				assert.Equal(t, keyword, rows[1][3])
				assert.Equal(t, fmt.Sprint(uid), rows[1][7])
			}
		})
	}
	t.Run("canceled_database_read", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("TMPDIR", tempDir)
		reqCtx, cancel := context.WithCancel(ctx)
		cancel()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tickets/export", nil).WithContext(reqCtx)
		c.Set("userID", uid)
		audit := &exportAudit{}
		handler.NewTicketHandler(svc, audit, nil).Export(c)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Empty(t, audit.entries)
		assert.Empty(t, rec.Header().Get("Content-Disposition"))
		entries, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
}
