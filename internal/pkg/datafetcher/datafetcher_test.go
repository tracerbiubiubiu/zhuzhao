package datafetcher

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubFetcher 测试用桩——回显 key + 固定结果集。
type stubFetcher struct {
	result map[string]any
	err    error
	calls  int
}

func (f *stubFetcher) Fetch(ctx context.Context, src FetchSource, key string, actor Actor) (map[string]any, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]any, len(f.result))
	for k, v := range f.result {
		out[k] = v
	}
	out["_key"] = key
	return out, nil
}

func TestRegistry_FailClosed(t *testing.T) {
	r := NewRegistry()

	t.Run("未注册 Type 拒执（fail-closed）", func(t *testing.T) {
		_, err := r.Fetch(context.Background(), FetchSource{Type: "unknown"}, "k", Actor{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown")
	})

	t.Run("空 Type 拒执", func(t *testing.T) {
		_, err := r.Fetch(context.Background(), FetchSource{Type: ""}, "k", Actor{})
		require.Error(t, err)
	})

	t.Run("已注册 Type 可执行", func(t *testing.T) {
		sf := &stubFetcher{result: map[string]any{"owner": "zhang"}}
		r.Register("internal_db", sf)
		out, err := r.Fetch(context.Background(), FetchSource{Type: "internal_db"}, "host1", Actor{UserID: 42})
		require.NoError(t, err)
		assert.Equal(t, "zhang", out["owner"])
		assert.Equal(t, "host1", out["_key"])
		assert.Equal(t, 1, sf.calls)
	})
}

func TestApplyFetches(t *testing.T) {
	r := NewRegistry()
	sf := &stubFetcher{result: map[string]any{"owner": "li", "org": "vg1"}}
	r.Register("internal_db", sf)

	schema := []FetchField{{
		Key:       "host_ip",
		Source:    FetchSource{Type: "internal_db", Config: json.RawMessage(`{}`)},
		TriggerBy: "host_ip",
		Mapping:   map[string]string{"owner": "responsible", "org": "org_name"},
	}}

	t.Run("正常取数回写", func(t *testing.T) {
		data := map[string]any{"host_ip": "10.0.0.1"}
		out, missed, err := ApplyFetches(r, schema, data, Actor{UserID: 1})
		require.NoError(t, err)
		assert.Empty(t, missed, "无缺失")
		assert.Equal(t, "li", out["responsible"], "映射回写 owner→responsible")
		assert.Equal(t, "vg1", out["org_name"])
		assert.Equal(t, "10.0.0.1", out["host_ip"], "原始字段保留")
	})

	t.Run("trigger 字段缺失→跳过+missed 标记", func(t *testing.T) {
		data := map[string]any{"other": "x"}
		out, missed, err := ApplyFetches(r, schema, data, Actor{UserID: 1})
		require.NoError(t, err)
		assert.Equal(t, []string{"host_ip"}, missed)
		assert.NotContains(t, out, "responsible", "目标字段留空")
	})

	t.Run("取数失败不阻塞（失败策略 A.4：留空+missed）", func(t *testing.T) {
		sfErr := &stubFetcher{err: errors.New("db down")}
		r2 := NewRegistry()
		r2.Register("internal_db", sfErr)
		data := map[string]any{"host_ip": "10.0.0.2"}
		out, missed, err := ApplyFetches(r2, schema, data, Actor{UserID: 1})
		require.NoError(t, err, "取数失败不阻塞提交")
		assert.Equal(t, []string{"host_ip"}, missed)
		assert.NotContains(t, out, "responsible", "目标字段留空")
	})

	t.Run("多字段取数——独立触发互不影响", func(t *testing.T) {
		schema2 := append(schema, FetchField{
			Key:       "ip2",
			Source:    FetchSource{Type: "internal_db"},
			TriggerBy: "second_ip",
			Mapping:   map[string]string{"owner": "second_owner"},
		})
		data := map[string]any{"host_ip": "a", "second_ip": "b"}
		out, missed, err := ApplyFetches(r, schema2, data, Actor{UserID: 1})
		require.NoError(t, err)
		assert.Empty(t, missed)
		assert.Equal(t, "li", out["responsible"])
		assert.Equal(t, "li", out["second_owner"])
	})
}
