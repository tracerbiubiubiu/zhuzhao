// CronRegistry cron 任务状态注册表（S10 B10-4 观测面——10 号 A.5）。
// 解装配环：wire_gen 建 handler 时注册表已存在，NewApp 装配完调度器再 Register
// （handler 读 Snapshot 实时取——注册表是壳，状态在调度器内并发安全快照）。
package service

import "sync"

// CronStatusProvider cron 状态提供者（*CronScheduler 天然实现）。
type CronStatusProvider interface {
	Status() CronStatus
}

// CronObserveFlagger 可选：观察模式标记（超时扫描专属——cron-status 透出，
// 运维确认首跑无异常后关闭）。
type CronObserveFlagger interface {
	Observe() bool
}

// CronRegistry 注册表（Register 仅装配期调用——非并发安全；Snapshot 并发安全）。
type CronRegistry struct {
	mu   sync.Mutex
	jobs map[string]CronStatusProvider
}

// NewCronRegistry 创建注册表。
func NewCronRegistry() *CronRegistry {
	return &CronRegistry{jobs: map[string]CronStatusProvider{}}
}

// Register 注册/覆盖一个任务状态提供者（nil 忽略——装配侧无需判空）。
func (r *CronRegistry) Register(name string, p CronStatusProvider) {
	if p == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[name] = p
}

// Snapshot 当前任务快照（map 拷贝——调用方安全遍历）。
func (r *CronRegistry) Snapshot() map[string]CronStatusProvider {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]CronStatusProvider, len(r.jobs))
	for k, v := range r.jobs {
		out[k] = v
	}
	return out
}
