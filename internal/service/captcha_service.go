package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// CaptchaService P4-7 登录验证码（Redis store 必须——规划口径）：
// 生成→SVG 自绘（零依赖）→Redis 一次性存储（GETDEL）→登录校验。
// enabled=false（dev/E2E 栈默认）时 Generate 返回开关态、Verify 恒通过——
// 登录契约零变化（前端按 GET /auth/captcha 响应显隐插槽）。
type CaptchaService struct {
	client  *redis.Client
	enabled bool
	ttl     time.Duration
}

func NewCaptchaService(client *redis.Client, enabled bool, ttl time.Duration) *CaptchaService {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &CaptchaService{client: client, enabled: enabled, ttl: ttl}
}

func (s *CaptchaService) Enabled() bool { return s != nil && s.enabled }

// 去混淆字符集（无 0/O/1/I/l）
const captchaAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randIntn(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// Generate 生成验证码：{captcha_id, image(base64 内联 SVG data URI), enabled}。
// enabled=false 时仅返回开关态（前端据此隐藏插槽——单端点两态，免独立 config 端点）。
func (s *CaptchaService) Generate(ctx context.Context) (map[string]any, error) {
	if !s.Enabled() {
		return map[string]any{"enabled": false}, nil
	}
	code := make([]byte, 4)
	for i := range code {
		code[i] = captchaAlphabet[randIntn(len(captchaAlphabet))]
	}
	id := uuid.NewString()
	if err := s.client.Set(ctx, "captcha:"+id, strings.ToUpper(string(code)), s.ttl).Err(); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	return map[string]any{
		"enabled":    true,
		"captcha_id": id,
		"image":      "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(renderSVG(string(code)))),
	}, nil
}

// Verify 一次性校验（GETDEL——用过即焚防重放；大小写不敏感）。
// enabled=false 恒通过；captcha_id 缺失/码错/过期 → ErrInvalidParams。
func (s *CaptchaService) Verify(ctx context.Context, captchaID, code string) error {
	if !s.Enabled() {
		return nil
	}
	if captchaID == "" || code == "" {
		return errcode.New(errcode.ErrInvalidParams.Code, "验证码必填")
	}
	stored, err := s.client.GetDel(ctx, "captcha:"+captchaID).Result()
	if err != nil {
		// 过期/不存在统一文案——不区分（防探测）；Redis 故障走 503
		if err == redis.Nil {
			return errcode.New(errcode.ErrInvalidParams.Code, "验证码已过期，请刷新后重试")
		}
		return errcode.ErrServiceUnavailable
	}
	if stored != strings.ToUpper(strings.TrimSpace(code)) {
		return errcode.New(errcode.ErrInvalidParams.Code, "验证码不正确")
	}
	return nil
}

// ---- SVG 自绘（标准库零依赖）----

// renderSVG 4 字符验证码图（120×40：随机 y 偏移+旋转+两条干扰折线+噪点）
func renderSVG(code string) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="40" viewBox="0 0 120 40">`)
	b.WriteString(`<rect width="120" height="40" fill="#f5f7fa"/>`)
	// 干扰折线 ×2
	for i := 0; i < 2; i++ {
		y0 := 6 + randIntn(28)
		b.WriteString(fmt.Sprintf(`<polyline points="4,%d 30,%d 60,%d 90,%d 116,%d" fill="none" stroke="#c0c4cc" stroke-width="1"/>`,
			y0, 6+randIntn(28), 6+randIntn(28), 6+randIntn(28), 6+randIntn(28)))
	}
	// 噪点 ×12
	for i := 0; i < 12; i++ {
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="1" fill="#909399"/>`, randIntn(120), randIntn(40)))
	}
	// 字符（随机色相深灰系+旋转±18°+y 抖动）
	colors := []string{"#409eff", "#67c23a", "#e6a23c", "#f56c6c", "#303133"}
	for i, ch := range code {
		x := 16 + i*26
		y := 26 + randIntn(6) - 3
		rot := randIntn(36) - 18
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" font-family="monospace" font-size="24" font-weight="bold" fill="%s" transform="rotate(%d %d %d)">%c</text>`,
			x, y, colors[randIntn(len(colors))], rot, x, y, ch))
	}
	b.WriteString(`</svg>`)
	return b.String()
}
