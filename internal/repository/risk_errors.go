package repository

import "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"

// 风险域错误码段 94000-94099（本批 D08/D09 启用 94001-94008）。
// 定义收口在 repository（SQL 判定直接返回）；service/risk 以别名引用，
// HTTP 映射见 handler/risk_errors.go——详设 risk-module-detail §2.2 错误面。
var (
	// 404：不可见与不存在同形（详设 §7——不泄露无权风险存在）。
	ErrRiskNotFound = errcode.New(94001, "风险不存在或不可见")
	// 400：已关闭/非法状态不接受评估等。
	ErrRiskInvalidState = errcode.New(94002, "风险当前状态不允许该操作")
	// 409：同一 operation_id 对应不同请求内容（详设 §2）。
	ErrRiskOperationConflict = errcode.New(94003, "同一操作编号对应不同请求内容")
	// 403：来源未登记、已停用（合成来源必须显式源身份与范围）。
	ErrRiskSourceUnknown = errcode.New(94004, "来源未登记或已停用")
	// 403：观察/调用超出该来源登记的授权范围。
	ErrRiskSourceScope = errcode.New(94005, "观察超出该来源授权范围")
	// 413：上报批次超过上限（详设 §2.1：100 条/批）。
	ErrRiskBatchTooLarge = errcode.New(94006, "上报批次超过上限")
	// 400：指派目标账号不存在或已停用。
	ErrRiskOwnerInvalid = errcode.New(94007, "责任人不存在或已停用")
	// 400：责任未确认——无法准备派单（第 7 项派单预览消费同一判据）。
	ErrRiskOwnerUnconfirmed = errcode.New(94008, "责任人未确认，无法准备派单")
)
