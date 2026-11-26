package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/singleton"
)

// PATGetter P4-6 PAT 查验出口（最小接口——repo 实现；nil=本实例不支持 PAT）
type PATGetter interface {
	FindActiveByHash(ctx context.Context, secretHash string) (userID int64, userName string, patID int64, err error)
	TouchLastUsed(ctx context.Context, patID int64)
}

// AccountSnapshot C05 认证入口的 DB 安全快照（账号事实判定不靠 Redis 缺省）。
type AccountSnapshot struct {
	Status          int
	Deleted         bool
	SessionEpoch    int64
	ProjectionEpoch int64
}

// AccountSnapshotter DB 当前账号事实读取出口（repo 实现）。
// 底座 §4.2 认证读顺序：先验 token 格式/签名/时效，再取本请求可信 DB 快照，
// 其后才核 Redis 会话约束——账号 inactive/deleted 是终态拒绝，DB 不可读是依赖 503。
type AccountSnapshotter interface {
	AccountSnapshot(ctx context.Context, userID int64) (AccountSnapshot, error)
}

type jwtOptions struct {
	accountSnapshots       AccountSnapshotter
	requireAccountSnapshot bool
}

// JWTOption JWT 中间件装配选项（保持既有三参调用方兼容——旧装配零改动）。
type JWTOption func(*jwtOptions)

// WithAccountSecurity requires a current DB account snapshot. A nil source fails
// closed; production routes always supply this option.
func WithAccountSecurity(src AccountSnapshotter) JWTOption {
	return func(o *jwtOptions) {
		o.accountSnapshots = src
		o.requireAccountSnapshot = true
	}
}

// JWT JWT 认证中间件（pats 非空时 Bearer zpat_* 走 PAT 查验——GitHub 形态直发）
func JWT(jwtManager *jwt.Manager, rdb *redis.Client, pats PATGetter, opts ...JWTOption) gin.HandlerFunc {
	o := &jwtOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return func(c *gin.Context) {
		if hasMixedAuth(c) {
			response.Error(c, http.StatusBadRequest, errcode.ErrMultipleAuthMethods)
			c.Abort()
			return
		}

		auth := c.GetHeader("Authorization")
		if auth == "" {
			// B4-1：仅带 X-AK-* 无 Bearer——M2M 未上线，明确告知（20009 待 Phase 3 落地）
			if hasAKHeaders(c) {
				response.Unauthorized(c, "暂不支持该认证方式")
				c.Abort()
				return
			}
			response.Unauthorized(c, errcode.ErrUnauthorized.Message)
			c.Abort()
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c, "无效的认证格式")
			c.Abort()
			return
		}

		tokenString := parts[1]

		// P4-6 PAT：zpat_ 前缀走独立查验（sha256 单查=命中+未吊销+未过期+用户未删）
		if pats != nil && strings.HasPrefix(tokenString, "zpat_") {
			uid, uname, patID, err := pats.FindActiveByHash(c.Request.Context(), HashToken(tokenString))
			if err != nil {
				response.UnauthorizedError(c, errcode.ErrTokenInvalid)
				c.Abort()
				return
			}
			if o.requireAccountSnapshot {
				if _, ok := currentAccountSnapshot(c, o, uid); !ok {
					return
				}
			}
			// 审计修复（2026-09-30 P2）：真异步+超时（原同步写——池耗尽时请求无限阻塞）
			go func(ctx context.Context, id int64) {
				tctx, tcancel := context.WithTimeout(ctx, 3*time.Second)
				defer tcancel()
				pats.TouchLastUsed(tctx, id)
			}(singleton.Detach(c.Request.Context()), patID)
			c.Set("userID", uid)
			c.Set("username", uname)
			c.Set("jti", fmt.Sprintf("pat:%d", patID))
			// 注记（审计二轮 P3）：强制改密态（疑似失陷处置）不传导到 PAT——GitHub PAT
			// 同款语义（跨密码重置存活）；处置失陷账号时须同步吊销其 PAT（运维 runbook 项）
			c.Set("must_change_password", false)
			// C05：PAT 校验后注入 pat_id（int64，C04 业务提交守卫消费凭据事实）
			c.Set("pat_id", patID)
			c.Next()
			return
		}

		claims, err := jwtManager.ParseAccessToken(tokenString)
		if err != nil {
			// B2-1：过期与无效区分——过期 → 20002（客户端可静默 refresh），
			// 签名错/typ 混淆/黑名单等 → 20003（需跳登录页）
			if errors.Is(err, jwt.ErrTokenExpired) {
				response.UnauthorizedError(c, errcode.ErrTokenExpired)
			} else {
				response.UnauthorizedError(c, errcode.ErrTokenInvalid)
			}
			c.Abort()
			return
		}

		// C05（底座 §4.2 认证读顺序）：Redis 判定之前先取本请求可信 DB 安全快照。
		// 账号 deleted → 20003 终态；inactive → 30003 终态；DB 不可读 → 503 依赖故障。
		// Redis 缺键不能替代 DB 判断——停用已提交而 Redis 撤销投影失败时，
		// 旧 AT 在此按 DB 当前事实终态拒绝（卡一首条红例）。
		var sessionEpoch int64
		if o.requireAccountSnapshot {
			snap, ok := currentAccountSnapshot(c, o, claims.UserID)
			if !ok {
				return
			}
			var err error
			sessionEpoch, err = positiveSessionEpoch(claims.SessionEpoch)
			if claims.SessionSchema != jwt.SessionSchemaDBEpochV1 || err != nil || sessionEpoch != snap.SessionEpoch {
				response.UnauthorizedError(c, errcode.ErrTokenInvalid)
				c.Abort()
				return
			}
			if !dbSessionProjectionReady(c, rdb, claims, snap) {
				return
			}
		}
		// C04 receives the verified signed epoch, never a generation borrowed from DB.
		c.Set("session_epoch", sessionEpoch)

		if !o.requireAccountSnapshot {
			// 黑名单 + disabled + 改密地板 + 密码纪元：一次 pipeline，逐命令 fail-close。
			blacklistKey := fmt.Sprintf("blacklist:at:%s", claims.JTI)
			disabledKey := fmt.Sprintf("user:disabled:%d", claims.UserID)
			atFloorKey := fmt.Sprintf("user:at_min_iat:%d", claims.UserID)
			pipe := rdb.Pipeline()
			blacklistCmd := pipe.Exists(c, blacklistKey)
			disabledCmd := pipe.Exists(c, disabledKey)
			atFloorCmd := pipe.Get(c, atFloorKey)
			epochCmd := pipe.Get(c, fmt.Sprintf("user:pw_epoch:%d", claims.UserID))
			// Pipeline.Exec 返回首个命令错误——GET 缺键即 redis.Nil（绝大多数用户无地板键），
			// 不是服务故障；此处只把真实错误判为 503，Nil 交由下方 atFloorCmd 逐命令判定
			//（2026-11-02 修：78733bf 引入 GET 后未过滤 Nil，致无地板键用户全量 503——
			// 单测 TestJWT_ValidToken_Passes 红即此症）。
			if _, err := pipe.Exec(c); err != nil && !errors.Is(err, redis.Nil) {
				response.ServiceUnavailable(c)
				c.Abort()
				return
			}
			if exists, err := blacklistCmd.Result(); err != nil {
				response.ServiceUnavailable(c)
				c.Abort()
				return
			} else if exists > 0 {
				response.UnauthorizedError(c, errcode.ErrTokenInvalid)
				c.Abort()
				return
			}
			if disabled, err := disabledCmd.Result(); err != nil {
				response.ServiceUnavailable(c)
				c.Abort()
				return
			} else if disabled > 0 {
				response.ForbiddenError(c, errcode.ErrUserDisabled)
				c.Abort()
				return
			}
			// B02-F01：撤销必须单调；秒级 iat 地板不能区分同秒新旧令牌，
			// disabled 被新登录清除后也不能让旧 AT 复活。缺键只兼容纪元 0。
			var epoch int64
			if raw, err := epochCmd.Result(); err == nil {
				var parseErr error
				epoch, parseErr = strconv.ParseInt(raw, 10, 64)
				if parseErr != nil || epoch < 0 {
					response.ServiceUnavailable(c)
					c.Abort()
					return
				}
			} else if !errors.Is(err, redis.Nil) {
				response.ServiceUnavailable(c)
				c.Abort()
				return
			}
			if claims.Pwe < 0 || claims.Pwe != epoch {
				response.UnauthorizedError(c, errcode.ErrTokenInvalid)
				c.Abort()
				return
			}

			// P1-D（2026-11-01 审计）：改密地板——签发早于地板的 AT 即时 401（与黑名单同语义）。
			// 修前自服务改密为放行新 AT 清除 disabled 键，其他设备旧 AT 复活残留 ≤AccessTTL；
			// 地板随 AccessTTL+60s 自然过期，新签 AT 的 iat ≥ 地板不受影响。Redis 故障同趟 fail-close。
			if floorStr, err := atFloorCmd.Result(); err == nil {
				if floor, perr := strconv.ParseInt(floorStr, 10, 64); perr == nil &&
					claims.IssuedAt != nil && claims.IssuedAt.Time.Unix() < floor {
					response.UnauthorizedError(c, errcode.ErrTokenInvalid)
					c.Abort()
					return
				}
			} else if !errors.Is(err, redis.Nil) {
				response.ServiceUnavailable(c)
				c.Abort()
				return
			}
		}

		// B4-1：FullPath 用路由模板比对（原硬编码 URL 字符串与注册解耦，
		// 前缀调整/trailing slash 时静默失效）
		if claims.MustChangePassword && c.FullPath() != "/api/v1/auth/password/update" {
			response.ForbiddenError(c, errcode.ErrPasswordChangeRequired)
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("jti", claims.JTI)
		c.Set("must_change_password", claims.MustChangePassword)
		if o.requireAccountSnapshot && claims.ExpiresAt != nil {
			c.Set(verifiedReadCredential, ReadCredential{UserID: claims.UserID, Epoch: sessionEpoch, JTI: claims.JTI, ExpiresAt: claims.ExpiresAt.Time})
		}

		c.Next()
	}
}

func currentAccountSnapshot(c *gin.Context, o *jwtOptions, userID int64) (AccountSnapshot, bool) {
	if o.accountSnapshots == nil {
		response.ServiceUnavailable(c)
		c.Abort()
		return AccountSnapshot{}, false
	}
	snap, err := o.accountSnapshots.AccountSnapshot(c.Request.Context(), userID)
	if err != nil {
		response.ServiceUnavailable(c)
		c.Abort()
		return AccountSnapshot{}, false
	}
	if snap.Deleted {
		response.UnauthorizedError(c, errcode.ErrTokenInvalid)
		c.Abort()
		return AccountSnapshot{}, false
	}
	if snap.Status != 1 {
		response.ForbiddenError(c, errcode.ErrUserDisabled)
		c.Abort()
		return AccountSnapshot{}, false
	}
	return snap, true
}

func positiveSessionEpoch(raw string) (int64, error) {
	epoch, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || epoch < 1 || strconv.FormatInt(epoch, 10) != raw {
		return 0, errors.New("invalid session epoch")
	}
	return epoch, nil
}

func dbSessionProjectionReady(c *gin.Context, rdb *redis.Client, claims *jwt.AccessClaims, snap AccountSnapshot) bool {
	credential := ReadCredential{UserID: claims.UserID, Epoch: snap.SessionEpoch, JTI: claims.JTI}
	revoked, ready := CheckReadSessionProjection(c.Request.Context(), rdb, credential, snap.ProjectionEpoch)
	if revoked {
		response.UnauthorizedError(c, errcode.ErrTokenInvalid)
		c.Abort()
		return false
	}
	if !ready {
		response.ServiceUnavailable(c)
		c.Abort()
	}
	return ready
}

const verifiedReadCredential = "verifiedReadCredential"

// ReadCredential contains the verified session facts needed by a bounded read.
// It carries neither the bearer token nor the user's password.
type ReadCredential struct {
	UserID    int64
	Epoch     int64
	JTI       string
	ExpiresAt time.Time
}

func AuthenticatedReadCredential(c *gin.Context) (ReadCredential, bool) {
	value, _ := c.Get(verifiedReadCredential)
	credential, ok := value.(ReadCredential)
	return credential, ok
}

// CheckReadSessionProjection reuses the normal login session projection checks
// for current authorization of an already authenticated read.
func CheckReadSessionProjection(ctx context.Context, rdb *redis.Client, credential ReadCredential, projectionEpoch int64) (revoked, ready bool) {
	if rdb == nil {
		return false, false
	}
	pipe := rdb.Pipeline()
	blacklist := pipe.Exists(ctx, "blacklist:at:"+credential.JTI)
	epoch := pipe.Get(ctx, fmt.Sprintf("user:session_epoch:db1:%d", credential.UserID))
	disabled := pipe.Exists(ctx, fmt.Sprintf("user:session_disabled:db1:%d", credential.UserID))
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return false, false
	}
	if revoked, err := blacklist.Result(); err != nil {
		return false, false
	} else if revoked > 0 {
		return true, false
	}
	raw, err := epoch.Result()
	projected, parseErr := positiveSessionEpoch(raw)
	marker, markerErr := disabled.Result()
	return false, err == nil && parseErr == nil && markerErr == nil && marker == 0 &&
		projectionEpoch == credential.Epoch && projected == credential.Epoch
}

func hasMixedAuth(c *gin.Context) bool {
	auth := c.GetHeader("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	return hasAKHeaders(c)
}

// hasAKHeaders 是否携带 AK/SK 请求头（M2M 认证，Phase 3 上线）
func hasAKHeaders(c *gin.Context) bool {
	return c.GetHeader("X-AK-Access-Key") != ""
}

// HashToken PAT 明文→sha256 hex（与 service.HashSecret 同算式——middleware 不引 service 防环）
func HashToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}
