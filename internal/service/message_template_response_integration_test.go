//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestTemplateWriteResponseIsActualPersistedRow(t *testing.T) {
	code := fmt.Sprint("response_", time.Now().UnixNano())
	defer func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM message_templates WHERE code=$1`, code)
	}()
	h := handler.NewMessageTemplateHandler(service.NewMessageTemplateService(repository.NewMessageTemplateRepo(testPool)))
	router := gin.New()
	router.POST("/create", h.Create)
	router.POST("/update", h.Update)
	post := func(path, body string) model.MessageTemplate {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code, w.Body.String())
		var env struct {
			Data model.MessageTemplate `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
		return env.Data
	}
	created := post("/create", fmt.Sprintf(`{"code":%q,"name":"original","channel":"uip","biz_type":"ticket","subject":"hello","body":"body","enabled":true,"id":"999999","version":99}`, code))
	require.Greater(t, created.ID, int64(0))
	require.NotEqualValues(t, 999999, created.ID)
	require.EqualValues(t, 1, created.Version)
	require.False(t, created.CreatedAt.IsZero())
	updated := post("/update", fmt.Sprintf(`{"code":%q,"name":"changed","channel":"webhook","biz_type":"forged-domain","subject":"hello 2","body":"body 2","enabled":true,"version":1}`, code))
	require.Equal(t, created.ID, updated.ID)
	require.EqualValues(t, 2, updated.Version)
	require.Equal(t, "uip", updated.Channel)
	require.Equal(t, "ticket", updated.BizType)
	require.Equal(t, created.CreatedAt, updated.CreatedAt)
}

func TestTemplateCreatorUsesTrustedHandlerPrincipal(t *testing.T) {
	actor := rbacTestUser(t, "template-actor-"+uniqueSuffix(), "TA"+uniqueSuffix(), nil)
	forged := rbacTestUser(t, "template-forged-"+uniqueSuffix(), "TF"+uniqueSuffix(), nil)
	code := "creator_" + uniqueSuffix()
	defer func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM message_templates WHERE code=$1`, code)
	}()
	h := handler.NewMessageTemplateHandler(service.NewMessageTemplateService(repository.NewMessageTemplateRepo(testPool)))
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", actor); c.Set("username", "actual-template-editor"); c.Next() })
	r.POST("/create", h.Create)
	r.POST("/update", h.Update)
	post := func(path string, version int) model.MessageTemplate {
		body := fmt.Sprintf(`{"code":%q,"name":"name","channel":"uip","biz_type":"ticket","subject":"subject","body":"body","enabled":true,"created_by":"%d","version":%d}`, code, forged, version)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code, w.Body.String())
		var env struct {
			Data model.MessageTemplate `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
		return env.Data
	}
	created := post("/create", 0)
	require.NotNil(t, created.CreatedBy)
	require.Equal(t, actor, *created.CreatedBy)
	updated := post("/update", 1)
	require.Equal(t, actor, *updated.CreatedBy)
}
