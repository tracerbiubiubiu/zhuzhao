// Package datafetcher 取数联动端口与注册表（B1-4——01 号附录 A.4 契约）。
//
// 按源三分治（S1-3 拍板 2026-10-21）：
//   - internal_db = 发起人身份（L2 范围内查询，取不到即留空——回写即探测防泄露通道）
//   - activelist = 服务身份（独立服务零权限检查，X-Operator 仅审计归因）
//   - asset-platform / external_http = 服务身份 + 非敏感白名单回填（X5 契约后细化）
//
// 失败策略（A.4 拍板）：取数失败不阻塞提交、目标字段留空 + missed 标记（调用方
// 记 linkage_missed 事件）——与分派失败同哲学。fail-closed：白名单外 Type 拒启/拒执。
package datafetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// FetchSource 取数源声明（A.4 契约形态）。
type FetchSource struct {
	Type   string          `json:"type"` // internal_db / activelist / external_http / asset-platform
	Config json.RawMessage `json:"config,omitempty"`
}

// Actor 取数身份（internal_db=发起人 UserID+L2 范围；服务身份源忽略 UserID 仅审计）。
type Actor struct {
	UserID int64
}

// DataFetcher 取数端口（A.4 契约——按 Type 注册，Fetch 按 key 查询返回键值集）。
type DataFetcher interface {
	Fetch(ctx context.Context, src FetchSource, key string, actor Actor) (map[string]any, error)
}

// FetchField 字段 schema 投影（field_meta.fetch 段——ticket 域组装）。
type FetchField struct {
	Key       string            // 触发字段 key（也是取数 key）
	Source    FetchSource       // 取数源
	TriggerBy string            // 触发字段名（通常 = Key；预留复合触发扩展）
	Mapping   map[string]string // 结果键 → 目标字段 key（如 "owner"→"responsible"）
}

// Registry 取数注册表（fail-closed：白名单外 Type 拒执）。
type Registry struct {
	mu       sync.RWMutex
	fetchers map[string]DataFetcher
}

func NewRegistry() *Registry {
	return &Registry{fetchers: map[string]DataFetcher{}}
}

// Register 注册 Type→Fetcher（重复注册覆盖——测试/热替换场景）。
func (r *Registry) Register(sourceType string, f DataFetcher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fetchers[sourceType] = f
}

// Types 已注册的 Type 清单（白名单面）。
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.fetchers))
	for t := range r.fetchers {
		out = append(out, t)
	}
	return out
}

// Fetch 按源取数（fail-closed：未注册 Type 即拒）。
func (r *Registry) Fetch(ctx context.Context, src FetchSource, key string, actor Actor) (map[string]any, error) {
	r.mu.RLock()
	f, ok := r.fetchers[src.Type]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("datafetcher: source type %q not registered (fail-closed)", src.Type)
	}
	return f.Fetch(ctx, src, key, actor)
}

// ApplyFetches 提交链取数回写（A.4：Submit 事务内、formula 重算之前）。
// 返回 = 回写后的 data 副本 + missed 字段清单（取数失败/触发缺失——调用方记事件）。
// 语义：目标字段**仅当不存在时才写入**（不覆盖用户已填值——auto_fill 建议预填
// 语义，提交信任用户值）；取数失败不阻塞、留空+missed 标记。
func ApplyFetches(r *Registry, schema []FetchField, data map[string]any, actor Actor) (map[string]any, []string, error) {
	if len(schema) == 0 {
		return data, nil, nil
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = v
	}
	var missed []string
	ctx := context.Background() // 提交链内联——ctx 由调用方透传更佳，首版固定
	for _, f := range schema {
		triggerKey := f.TriggerBy
		if triggerKey == "" {
			triggerKey = f.Key
		}
		triggerVal, has := data[triggerKey]
		if !has || triggerVal == nil || triggerVal == "" {
			missed = append(missed, f.Key)
			continue
		}
		result, err := r.Fetch(ctx, f.Source, fmt.Sprintf("%v", triggerVal), actor)
		if err != nil || result == nil {
			missed = append(missed, f.Key)
			continue
		}
		for srcKey, dstKey := range f.Mapping {
			if v, ok := result[srcKey]; ok {
				if _, exists := out[dstKey]; !exists {
					out[dstKey] = v // 仅当不存在时写入——不覆盖用户已填值
				}
			}
		}
	}
	return out, missed, nil
}
