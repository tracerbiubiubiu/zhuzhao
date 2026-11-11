//go:build wireinject
// +build wireinject

package app

import (
	"github.com/google/wire"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// InitializeApp Wire 注入入口（薄壳——B 方案全量修 2026-10-24）。
// provideApp 为唯一 provider：手码装配全部集中在 providers.go 的 provideApp 内，
// wire.go 只声明入口。wire 再生产出的 wire_gen.go 仅调 provideApp 一行——
// 无论内部手码怎么改，wire 再生永远可重现（零漂移守卫可入 CI）。
func InitializeApp(cfg *config.Config) (*App, func(), error) {
	wire.Build(provideApp)
	return nil, nil, nil
}
