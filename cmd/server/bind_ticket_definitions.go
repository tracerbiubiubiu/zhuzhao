package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/admincli"
)

const bindTicketDefinitionsUsage = `用法：zhuzhao bind-ticket-definitions < approved-manifest.json

由具备数据库写入权限的本地运维执行。从标准输入读取单票 JSON 对象，向标准输出写入 JSON 回执。
执行前须在获准环境核实服务配置指向的数据库。审批应覆盖该数据库、完整旧规则、原票事实及此票到指定定义的映射。
证据摘要用于回执追溯，证据核验和批准在命令外完成。清单只填写下列字符串字段。

  ticket_id, original_created_by, definition_version  无前导零的正整数十进制字符串
  original_created_at                                 RFC3339，精确到微秒
  type_code, expected_status                          原票类型代码和预期状态
  definition_hash                                     以旧规则证据核算的完整 format-1 定义摘要
  configuration_evidence_sha256                       迁移前完整配置证据摘要
  mapping_evidence_sha256                              显式票到定义映射证据摘要
  approval_sha256                                      获准执行的证据摘要
  ticket_evidence_sha256                               原票事实证据摘要

摘要为 64 位小写十六进制。根据独立核准的旧证据整理完整 format-1 定义，使用 PostgreSQL 函数
ticket_definition_snapshot_hash(jsonb) 计算摘要。
保留状态、转换规则与元数据、SLA 及全部字段。字段顺序和 field_id 必须有可核实的对应关系，
行 ID、当前发布指针及其他运行元数据按 format-1 归一。旧规则或字段身份无法核实时停止执行。
目标定义必须已存储，类型、版本及摘要必须与清单一致。证据原件留在获准的归档中。
重新提交相同清单可取回原补绑回执。
`

func runBindTicketDefinitions(args []string) error {
	if len(args) != 0 {
		fmt.Fprint(os.Stderr, bindTicketDefinitionsUsage)
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			return nil
		}
		return errors.New("请从标准输入提交 JSON 清单")
	}

	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		return errors.New("加载服务配置失败")
	}
	// 本地运维命令只使用数据库配置。
	if cfg.Database.Host == "" || cfg.Database.Port <= 0 || cfg.Database.User == "" || cfg.Database.DBName == "" {
		return errors.New("配置缺少 database 必要字段")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return admincli.RunBindTicketDefinitions(ctx, cfg.Database.DSN(), os.Stdin, os.Stdout)
}
