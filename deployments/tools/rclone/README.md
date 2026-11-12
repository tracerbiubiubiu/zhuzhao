# rclone 工具镜像

固定官方 `github.com/rclone/rclone v1.76.0`，仅将有已确认安全公告的 `golang.org/x/crypto` 提升到 v0.59.0，并使用 Go1.27.4 重新构建。`go.mod`/`go.sum` 是源码和依赖锁，不维护 fork，不使用 `replace`，不裁剪 rclone 命令或 backend。此目录没有自写 Go 程序，构建目标直接指向上游 main 包；不要对空 wrapper 运行 `go mod tidy`，以免删除明确保留的依赖。

官方 1.76.0 镜像的旧 Go 构建版本和裁剪过的 ELF 不能作为无漏洞证据。本镜像固定 Go/Alpine OCI 摘要，启用 checksum database，保留符号和调试信息；在实际 main 包与成品 ELF 上重新扫描。运行层保留 shell、CA、时区、FUSE、mime 数据和上游 rclone 用户，兼容现有 S3 备份脚本。

从主仓根目录构建：

```bash
docker build -t zzu-upgrade-rclone:20261112 deployments/tools/rclone
docker run --rm zzu-upgrade-rclone:20261112 version
```

`GOPROXY` 构建参数默认使用官方 Go proxy，可指向提供相同已校验模块的缓存；不关闭 `GOSUMDB=sum.golang.org`。构建不依赖仓库根 `go.mod`、内部模块或已有业务二进制。离线运行环境应导入预构建镜像，不在运行时下载源码或 SDK。本地 tag 是验收标记，正式制品由统一镜像锁记录摘要。

源码扫描必须指向实际 main 包，不能扫描空 wrapper 后宣称 rclone 无漏洞：

```bash
# 在本目录，使用 Go1.27.4 与 govulncheck v1.9.0
CGO_ENABLED=0 GOWORK=off GOFLAGS='-mod=readonly -p=2' \
  govulncheck -mode=source -scan=symbol -test=false \
  github.com/rclone/rclone
```

还须从实际 Docker 镜像提取 `/usr/local/bin/rclone`，以默认 text 格式进行 binary scan 并严格检查退出码。JSON 模式只作证据收集，不能据 scanner exit0 判安全通过。不得把 stripped binary 的 OSV 符号展开误称为实际符号或运行时可达性证据。

功能门禁使用独立 RustFS 合成 bucket，原样执行 `deployments/backup/rustfs-mirror.sh` 至少两轮，验证初次同步、覆盖/删除前对象的 `--backup-dir` 留存和从归档恢复的逐字节一致；不改原备份脚本行为，不触碰共享对象数据。

`/usr/share/licenses/rclone/` 保留实际构建依赖的上游 LICENSE/NOTICE/COPYING/COPYRIGHT 文件及 Go LICENSE，复制失败会阻断构建；`missing-root-license.txt` 列出没有匹配到常规根目录许可文件的模块。`/usr/share/doc/rclone/` 包含模块版本清单和编译器、源码版本、ELF 摘要信息。当前 S3 备份不启用 RC/serve；以后启用服务接口或其他接入能力时，仍需按真实使用路径重新评估安全边界。
