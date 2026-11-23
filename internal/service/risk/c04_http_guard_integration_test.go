//go:build integration

package risk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

// C04（F017）：HTTP 写命令在守卫事务内重验当前凭据与当前 L1 授权。
// 中间件用「内存策略仍在」的 enforcer 放行请求，撤权只落在 DB——
// 服务端写事务内的 fresh 重验必须拒绝，以此证明重验读的是库内现势事实。
func TestRiskAssessCommand_ReverifiesCredentialAndPolicyInTransaction(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := risk.NewService(testPool, nil)

	for _, variant := range []string{"happy", "account_disabled", "policy_revoked_db", "pat_revoked"} {
		t.Run(variant, func(t *testing.T) {
			roleCode, actor := seedCasbinRole(t, "/api/v1/risk-items/assess")
			seed := mustCreate(t, svc, "op-c04-create-"+variant+"-"+uniqueSuffix(), "C04-"+uniqueSuffix(), orgGID, &userU)

			var patID int64
			require.NoError(t, testPool.QueryRow(ctx,
				`INSERT INTO personal_access_tokens(user_id,name,secret_hash)VALUES($1,'c04-fixture',$2)RETURNING id`,
				actor, fmt.Sprintf("%064x", time.Now().UnixNano())).Scan(&patID))

			// 变体前置：撤权只写 DB；中间件内存策略不受影响。
			switch variant {
			case "account_disabled":
				_, err := testPool.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, actor)
				require.NoError(t, err)
			case "policy_revoked_db":
				_, err := testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v2='POST' AND v1='/api/v1/risk-items/assess'`, "role::"+roleCode)
				require.NoError(t, err)
			case "pat_revoked":
				_, err := testPool.Exec(ctx, `UPDATE personal_access_tokens SET revoked_at=NOW() WHERE id=$1`, patID)
				require.NoError(t, err)
			}

			m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
			require.NoError(t, err)
			enforcer, err := casbin.NewSyncedEnforcer(m)
			require.NoError(t, err)
			_, err = enforcer.AddPolicy("role::"+roleCode, "/api/v1/risk-items/assess", "POST")
			require.NoError(t, err)
			rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool), repository.NewMenuRepo(testPool), enforcer)

			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set("userID", actor)
				c.Set("pat_id", patID)
				c.Next()
			})
			r.Use(middleware.CasbinAuth(enforcer, rbac, nil))
			r.POST("/api/v1/risk-items/assess", handler.NewRiskHandler(svc, nil).Assess)

			body, err := json.Marshal(model.RiskAssessRequest{
				OperationID: "op-c04-" + variant + "-" + uniqueSuffix(), ID: seed.Risk.ID,
				ExpectedRevision: "1", Level: "high", Treatment: "mitigate",
				AssessmentNote: "C04 in-tx recheck",
			})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/risk-items/assess", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			switch variant {
			case "happy":
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				var rev int64
				require.NoError(t, testPool.QueryRow(ctx,
					`SELECT revision FROM risk_items WHERE id=$1`, seed.Risk.ID).Scan(&rev))
				require.Equal(t, int64(2), rev)
			default:
				require.Equal(t, http.StatusForbidden, rec.Code,
					"variant %s 应因库内现势撤权被拒: %s", variant, rec.Body.String())
				var rev int64
				require.NoError(t, testPool.QueryRow(ctx,
					`SELECT revision FROM risk_items WHERE id=$1`, seed.Risk.ID).Scan(&rev))
				require.Equal(t, int64(1), rev, "拒绝后业务数据不变")
			}
		})
	}
}
