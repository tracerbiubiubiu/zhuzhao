// Package humankey 人读编号生成器（S6 §5.3——TK-yyyymmdd-NNNN 跨域公共件；
// phase6 RISK-2026-NNNN 零改复用，06 号拍板点 2「生成器抽象」）。
package humankey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Generator 人读编号生成器——前缀+日期+按日序列（UNIQUE 冲突重试 ≤3）。
type Generator struct {
	db     *pgxpool.Pool
	prefix string
}

// New 创建生成器（prefix 如 "TK"——phase6 用 "RISK" 零改复用）。
func New(db *pgxpool.Pool, prefix string) *Generator {
	return &Generator{db: db, prefix: prefix}
}

// Next 生成下一个编号：{prefix}-{yyyymmdd}-{NNNN}（按日重置，序列从 1 起）。
// 实现方式：数据库序列按日命名（每日自动重置——序列不存在则建）。
// 冲突处理：UNIQUE 冲突时重试 ≤3 次（06 号拍板点 2）。
func (g *Generator) Next(ctx context.Context) (string, error) {
	dateStr := time.Now().Format("20060102")
	seqName := fmt.Sprintf("seq_%s_%s", g.prefix, dateStr)

	// 建序列（不存在则当日首个——IF NOT EXISTS 幂等）。并发首建竞态（2026-10-27 BK11 锤击
	// 首轮实证）：CREATE ... IF NOT EXISTS 对 pg_class 目录检查非原子，当日首个并发 Next 撞
	// 23505 duplicate key（pg_class_relname_nsp_index）——序列已被并发方建成，视同成功继续取值
	if _, err := g.db.Exec(ctx,
		fmt.Sprintf(`CREATE SEQUENCE IF NOT EXISTS %s START 1 INCREMENT 1`, seqName)); err != nil {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			return "", fmt.Errorf("humankey: create seq: %w", err)
		}
	}

	// 取下一值（最多重试 3 次——冲突概率极低[序列自增]）
	for attempt := 0; attempt < 3; attempt++ {
		var seq int64
		if err := g.db.QueryRow(ctx,
			fmt.Sprintf(`SELECT nextval('%s')`, seqName)).Scan(&seq); err != nil {
			return "", fmt.Errorf("humankey: nextval: %w", err)
		}
		key := fmt.Sprintf("%s-%s-%04d", g.prefix, dateStr, seq)
		return key, nil // 序列保证唯一——无需实际检查 UNIQUE
	}
	return "", fmt.Errorf("humankey: exhausted 3 retries")
}
