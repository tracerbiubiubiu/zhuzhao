package risk

import "github.com/tracerbiubiubiu/zhuzhao/internal/repository"

// 风险域错误别名（码定义与段注释见 repository/risk_errors.go——SQL 判定处产生；
// handler 映射见 handler/risk_errors.go）。
var (
	ErrRiskNotFound          = repository.ErrRiskNotFound
	ErrRiskInvalidState      = repository.ErrRiskInvalidState
	ErrRiskOperationConflict = repository.ErrRiskOperationConflict
	ErrRiskSourceUnknown     = repository.ErrRiskSourceUnknown
	ErrRiskSourceScope       = repository.ErrRiskSourceScope
	ErrRiskBatchTooLarge     = repository.ErrRiskBatchTooLarge
	ErrRiskOwnerInvalid      = repository.ErrRiskOwnerInvalid
	ErrRiskOwnerUnconfirmed  = repository.ErrRiskOwnerUnconfirmed
)
