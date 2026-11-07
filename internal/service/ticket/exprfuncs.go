// B1-6 批2：表达式函数注册表装配件（11 号 §1 B1-6 行——A.3 函数白名单实施
// 形态：DisableAllBuiltins 后零内置，计算能力=AddFunction 注册 Go 函数）。
package ticket

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/exprcalc"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type dictQuerierKey struct{}

// withDictQuerier 仅在本轮求值绑定已有读通道（Update=tx，前置校验=pool）。
// 派生上下文不传给事件、worker 或下一事务。
func withDictQuerier(ctx context.Context, q repository.RowQuerier) context.Context {
	return context.WithValue(ctx, dictQuerierKey{}, q)
}

// RegisterExprDictLookup 注册进程级函数定义；请求/事务读通道仅在求值时传入。
// Service 总是绑定当前读通道；pool 仅作直接调用表达式的兼容后备，仍使用求值 ctx。
func RegisterExprDictLookup(pool *pgxpool.Pool) {
	exprcalc.AddFunction("lookup_dict", func(params ...any) (any, error) {
		if len(params) != 3 {
			return nil, fmt.Errorf("lookup_dict(code, item) 须两参")
		}
		ctx, ok := params[0].(context.Context)
		if !ok || ctx == nil {
			return nil, fmt.Errorf("lookup_dict 缺少求值上下文")
		}
		typeCode, ok1 := params[1].(string)
		itemCode, ok2 := params[2].(string)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("lookup_dict 两参均须字符串")
		}
		q := repository.RowQuerier(pool)
		if current, ok := ctx.Value(dictQuerierKey{}).(repository.RowQuerier); ok {
			q = current
		}
		rows, err := q.Query(ctx,
			`SELECT label FROM dict_items WHERE type_code=$1 AND code=$2 AND enabled=true`, typeCode, itemCode)
		if err == nil {
			// Match QueryRow.Scan: finish the read and inspect its final error
			// before returning the label, including cancellation during Close.
			label, readErr := pgx.CollectOneRow(rows, pgx.RowTo[string])
			if readErr == nil {
				return label, nil
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("lookup_dict(%q, %q) 未命中启用项", typeCode, itemCode)
	}, new(func(context.Context, string, string) (string, error)))
}
