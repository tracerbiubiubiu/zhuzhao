# migrate 工具镜像

固定官方 `github.com/golang-migrate/migrate/v4 v4.20.1`，用 Go1.27.4 重新构建。`go.mod`/`go.sum` 是源码和依赖锁，不维护 fork，不使用 `replace`。此目录没有自写 Go 程序，构建目标直接指向上游 `cmd/migrate`；不要在这里对空模块运行 `go mod tidy`，以免删除明确保留的上游依赖。

只使用 `postgres` build tag，注册 `postgres://` 与 `postgresql://` 的 lib/pq 驱动。`file` 源由上游 `internal/cli/commands.go` 无条件导入，另保留其内置 `stub` 注册。当前生态的本地迁移文件和 PostgreSQL DSN 均在范围内；MySQL、pgx、云存储源等其他驱动没有编入该专用镜像，不将其冒充上游全驱动发行包。

上游镜像即使是最新 tag，仍可能使用旧编译器并包含当前不用的全套驱动依赖。本镜像使用固定 Go/Alpine OCI 摘要、校验模块和、保持 `GOSUMDB=sum.golang.org`，并保留 ELF 符号与调试信息，供实际二进制扫描使用。运行层保留 shell、CA 和时区数据，兼容现有迁移入口。

从主仓根目录构建：

```bash
docker build -t zzu-upgrade-migrate:20261112 deployments/tools/migrate
docker run --rm zzu-upgrade-migrate:20261112 -version
docker run --rm zzu-upgrade-migrate:20261112 -help
```

`GOPROXY` 构建参数默认使用官方 Go proxy，可指向提供相同已校验模块的缓存；不关闭 checksum database。镜像构建不依赖仓库根 `go.mod`、内部模块或已有业务二进制。离线运行环境应导入预构建镜像，启动迁移时无需下载源码或 SDK。本地 tag 是验收标记，正式制品由统一镜像锁记录摘要。

源码扫描必须使用实际 main 包和同一 tag，不能扫描空 wrapper 后宣称工具无漏洞：

```bash
# 在本目录，使用 Go1.27.4 与 govulncheck v1.9.0
CGO_ENABLED=0 GOWORK=off GOFLAGS='-mod=readonly -p=2' \
  govulncheck -mode=source -scan=symbol -test=false -tags=postgres \
  github.com/golang-migrate/migrate/v4/cmd/migrate
```

还须从实际 Docker 镜像提取 `/usr/local/bin/migrate`，以默认 text 格式执行 binary scan 并严格检查退出码。JSON 模式仅作证据收集，不能据 scanner exit0 判安全通过。功能门禁是独立 PG18 数据库的全部 81 迁移 `up → down -all → up`，以及往返后的 schema 对比；不得连到共享开发库进行该演练。

`/usr/share/licenses/migrate/` 保留实际构建依赖的上游 LICENSE/NOTICE/COPYING/COPYRIGHT 文件及 Go LICENSE，复制失败会阻断构建；`missing-root-license.txt` 列出没有匹配到常规根目录许可文件的模块。`/usr/share/doc/migrate/` 包含模块版本清单和编译器、源码版本、ELF 摘要信息。更新时同时核验上游来源、锁文件、Docker 版本标记和这些验收证据。
