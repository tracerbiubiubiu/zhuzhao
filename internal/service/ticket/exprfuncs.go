// B1-6 批2：表达式函数注册表装配件（11 号 §1 B1-6 行——A.3 函数白名单实施
// 形态：DisableAllBuiltins 后零内置，计算能力=AddFunction 注册 Go 函数）。
package ticket

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/exprcalc"
)

// RegisterExprDictLookup 装配期注册 lookup_dict（db 闭包注入——exprcalc 为
// 纯包不持连接；查 dict_items 启用项取 label，未命中=求值错误→提交链 400）。
func RegisterExprDictLookup(pool *pgxpool.Pool) {
	exprcalc.AddFunction("lookup_dict", func(params ...any) (any, error) {
		if len(params) != 2 {
			return nil, fmt.Errorf("lookup_dict(code, item) 须两参")
		}
		typeCode, ok1 := params[0].(string)
		itemCode, ok2 := params[1].(string)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("lookup_dict 两参均须字符串")
		}
		var label string
		err := pool.QueryRow(context.Background(),
			`SELECT label FROM dict_items WHERE type_code=$1 AND code=$2 AND enabled=true`, typeCode, itemCode).Scan(&label)
		if err != nil {
			return nil, fmt.Errorf("lookup_dict(%q, %q) 未命中启用项", typeCode, itemCode)
		}
		return label, nil
	})
}
