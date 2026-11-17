package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const maintenanceAction = "audit_archive"

type taskPrincipalKey struct{}
type taskPrincipal struct {
	ID   int64
	Name string
}

// WithTaskPrincipal is for authenticated app adapters, never body/header values.
// The stable ID grants nothing by itself: maintenance also needs explicit startup
// authority and a current active database user. Name is attribution only.
func WithTaskPrincipal(ctx context.Context, id int64, name string) context.Context {
	return context.WithValue(ctx, taskPrincipalKey{}, taskPrincipal{ID: id, Name: name})
}
func taskPrincipalOf(ctx context.Context) taskPrincipal {
	p, _ := ctx.Value(taskPrincipalKey{}).(taskPrincipal)
	return p
}
func taskCreator(ctx context.Context) string {
	p := taskPrincipalOf(ctx)
	if p.ID <= 0 {
		return "service:zhuzhao"
	}
	if p.Name != "" {
		return p.Name
	}
	return strconv.FormatInt(p.ID, 10)
}

type maintenanceWindow struct {
	From, Until time.Time
	Reason      string
}
type maintenanceOverride struct {
	maintenanceWindow
	UserID int64
	Days   int
}
type maintenanceFixedJob struct {
	maintenanceWindow
	Digest string
}
type maintenanceExecutionKey struct{}
type maintenanceExecution struct {
	policy    *TaskActionPolicy
	admission repository.MaintenanceAdmission
}

type TaskActionPolicy struct {
	db          *pgxpool.Pool
	repo        *repository.MaintenanceAdmissionRepo
	client      *taskrunner.Client
	defaultDays int
	maintainers map[int64]bool
	overrides   map[string]maintenanceOverride
	fixed       map[string]maintenanceFixedJob
}

func NewTaskActionPolicy(db *pgxpool.Pool, cfg config.MaintenanceConfig, defaultDays int, client *taskrunner.Client) (*TaskActionPolicy, error) {
	if defaultDays <= 0 {
		defaultDays = 180
	}
	p := &TaskActionPolicy{db: db, repo: repository.NewMaintenanceAdmissionRepo(db), client: client, defaultDays: defaultDays, maintainers: map[int64]bool{}, overrides: map[string]maintenanceOverride{}, fixed: map[string]maintenanceFixedJob{}}
	for _, id := range cfg.MaintainerUserIDs {
		if id <= 0 || p.maintainers[id] {
			return nil, fmt.Errorf("maintenance maintainer_user_ids must be unique positive IDs")
		}
		p.maintainers[id] = true
	}
	for _, o := range cfg.Overrides {
		w, err := parseMaintenanceWindow(o.ValidFrom, o.ValidUntil, o.Reason)
		if err != nil {
			return nil, err
		}
		if o.ID == "" || len(o.ID) > 64 || !p.maintainers[o.UserID] || o.RetentionDays <= 0 || o.RetentionDays > 36500 {
			return nil, fmt.Errorf("maintenance override must bind an explicit maintainer, bounded retention and approval ID")
		}
		if _, ok := p.overrides[o.ID]; ok {
			return nil, fmt.Errorf("maintenance override IDs must be unique")
		}
		p.overrides[o.ID] = maintenanceOverride{maintenanceWindow: w, UserID: o.UserID, Days: o.RetentionDays}
	}
	for _, f := range cfg.FixedJobs {
		w, err := parseMaintenanceWindow(f.ValidFrom, f.ValidUntil, f.Reason)
		if err != nil {
			return nil, err
		}
		digest, err := hex.DecodeString(f.DefinitionSHA256)
		if f.JobID == "" || len(digest) != sha256.Size || err != nil {
			return nil, fmt.Errorf("fixed maintenance job requires job_id and SHA256 of the reviewed definition")
		}
		if _, ok := p.fixed[f.JobID]; ok {
			return nil, fmt.Errorf("fixed maintenance job IDs must be unique")
		}
		p.fixed[f.JobID] = maintenanceFixedJob{maintenanceWindow: w, Digest: strings.ToLower(f.DefinitionSHA256)}
	}
	return p, nil
}
func parseMaintenanceWindow(from, until, reason string) (maintenanceWindow, error) {
	a, e1 := time.Parse(time.RFC3339Nano, from)
	b, e2 := time.Parse(time.RFC3339Nano, until)
	a = a.UTC().Truncate(time.Microsecond)
	b = b.UTC().Truncate(time.Microsecond)
	if e1 != nil || e2 != nil || !a.Before(b) || strings.TrimSpace(reason) == "" || len(reason) > 500 {
		return maintenanceWindow{}, fmt.Errorf("maintenance approval requires an explicit valid_from/valid_until window and bounded reason")
	}
	return maintenanceWindow{From: a, Until: b, Reason: reason}, nil
}
func (w maintenanceWindow) active(now time.Time) bool {
	return !now.Before(w.From) && now.Before(w.Until)
}

// ValidateArchiveParams rejects ambiguous JSON, unknown keys and unapproved
// retention overrides. Zero/missing retains the old default-retention meaning.
func decodeArchiveParams(raw []byte) (archiveParams, error) {
	if len(raw) > 64<<10 {
		return archiveParams{}, errcode.ErrInvalidParams
	}
	if _, err := jobs.EqualParams(raw, raw); err != nil {
		return archiveParams{}, errcode.ErrInvalidParams
	}
	data := bytes.TrimSpace(raw)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		data = []byte("{}")
	}
	if data[0] != '{' {
		return archiveParams{}, errcode.ErrInvalidParams
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return archiveParams{}, errcode.ErrInvalidParams
	}
	if err := canonicalTaskFields(keys, "retention_days", "approval_id"); err != nil {
		return archiveParams{}, err
	}
	if v, ok := keys["retention_days"]; ok && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		return archiveParams{}, errcode.ErrInvalidParams
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var params archiveParams
	if err := d.Decode(&params); err != nil {
		return archiveParams{}, errcode.ErrInvalidParams
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return archiveParams{}, errcode.ErrInvalidParams
	}
	if params.RetentionDays < 0 || params.RetentionDays > 36500 || len(params.ApprovalID) > 64 {
		return archiveParams{}, errcode.ErrInvalidParams
	}
	return params, nil
}
func (p *TaskActionPolicy) manualFact(ctx context.Context, taskID string, params json.RawMessage) (*repository.MaintenanceAdmission, error) {
	if p == nil {
		return nil, errcode.ErrForbidden
	}
	principal := taskPrincipalOf(ctx)
	if principal.ID <= 0 || !p.maintainers[principal.ID] {
		return nil, errcode.ErrForbidden
	}
	parsed, err := decodeArchiveParams(params)
	if err != nil {
		return nil, err
	}
	days := parsed.RetentionDays
	if days == 0 {
		days = p.defaultDays
	}
	a := &repository.MaintenanceAdmission{TaskID: taskID, Action: maintenanceAction, Params: normalizedTaskParams(params), Origin: "manual", PrincipalUserID: principal.ID, RetentionDays: days, Reason: "explicit startup maintainer; configured default retention"}
	if parsed.ApprovalID != "" {
		o, ok := p.overrides[parsed.ApprovalID]
		if !ok || o.UserID != principal.ID || o.Days != days || !o.active(time.Now()) {
			return nil, errcode.ErrForbidden
		}
		a.ApprovalID, a.Reason = parsed.ApprovalID, o.Reason
		a.ValidFrom = &o.From
		a.ValidUntil = &o.Until
	} else if days != p.defaultDays {
		return nil, errcode.ErrForbidden
	}
	if err := p.activeUser(ctx, principal.ID, nil); err != nil {
		return nil, err
	}
	return a, nil
}
func normalizedTaskParams(raw []byte) string {
	data := bytes.TrimSpace(raw)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return "{}"
	}
	return string(data)
}
func (p *TaskActionPolicy) activeUser(ctx context.Context, id int64, tx pgx.Tx) error {
	if p == nil || p.db == nil {
		return errcode.ErrServiceUnavailable
	}
	var active bool
	q := `SELECT status=1 AND deleted_at IS NULL FROM users WHERE id=$1`
	var err error
	if tx != nil {
		err = tx.QueryRow(ctx, q+" FOR SHARE", id).Scan(&active)
	} else {
		err = p.db.QueryRow(ctx, q, id).Scan(&active)
	}
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !active {
		return errcode.ErrForbidden
	}
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	return nil
}
func sameMaintenanceFact(a, b *repository.MaintenanceAdmission) bool {
	equal, err := jobs.EqualParams([]byte(a.Params), []byte(b.Params))
	sameTime := func(a, b *time.Time) bool {
		if a == nil || b == nil {
			return a == nil && b == nil
		}
		return a.Equal(*b)
	}
	return err == nil && equal && a.Action == b.Action && a.Origin == b.Origin && a.PrincipalUserID == b.PrincipalUserID && a.JobID == b.JobID && a.DefinitionSHA256 == b.DefinitionSHA256 && a.RetentionDays == b.RetentionDays && a.ApprovalID == b.ApprovalID && a.Reason == b.Reason && sameTime(a.ValidFrom, b.ValidFrom) && sameTime(a.ValidUntil, b.ValidUntil)
}
func (p *TaskActionPolicy) prepareTask(ctx context.Context, id, action string, params json.RawMessage) error {
	if action != maintenanceAction {
		return nil
	}
	a, err := p.manualFact(ctx, id, params)
	if err != nil {
		return err
	}
	old, err := p.repo.Put(ctx, a)
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	if !sameMaintenanceFact(a, old) {
		return errcode.ErrConflict
	}
	return nil
}
func (p *TaskActionPolicy) authorizeAction(ctx context.Context, action string, params json.RawMessage) error {
	if action != maintenanceAction {
		return nil
	}
	_, err := p.manualFact(ctx, "", params)
	return err
}

// AuthorizeCallback never trusts callback actor/created_by. Unknown old sensitive
// rows remain denied. Fixed cron facts require an actual TR run, a reviewed exact
// definition, and enqueue after the configuration's approval window began.
func (p *TaskActionPolicy) AuthorizeCallback(ctx context.Context, taskID, action string, params json.RawMessage) (context.Context, error) {
	if action != maintenanceAction {
		return ctx, nil
	}
	if p == nil {
		return ctx, errcode.ErrForbidden
	}
	if p.db == nil {
		return ctx, errcode.ErrServiceUnavailable
	}
	if _, err := decodeArchiveParams(params); err != nil {
		return ctx, err
	}
	a, err := p.repo.Get(ctx, taskID)
	if errors.Is(err, repository.ErrMaintenanceAdmissionNotFound) {
		a, err = p.fixedFact(ctx, taskID, params)
		if err == nil {
			var old *repository.MaintenanceAdmission
			old, err = p.repo.Put(ctx, a)
			if err == nil && !sameMaintenanceFact(a, old) {
				err = errcode.ErrConflict
			}
			a = old
		}
	}
	if err != nil {
		if errors.Is(err, errcode.ErrForbidden) || errors.Is(err, errcode.ErrConflict) {
			return ctx, err
		}
		return ctx, errcode.ErrServiceUnavailable
	}
	equal, err := jobs.EqualParams([]byte(a.Params), params)
	if err != nil || !equal || a.Action != action {
		return ctx, errcode.ErrForbidden
	}
	if a.Origin == "manual" {
		current, err := p.manualFact(WithTaskPrincipal(ctx, a.PrincipalUserID, ""), taskID, params)
		if err != nil {
			return ctx, err
		}
		if !sameMaintenanceFact(a, current) {
			return ctx, errcode.ErrForbidden
		}
	} else {
		current, err := p.fixedFact(ctx, taskID, params)
		if err != nil {
			return ctx, err
		}
		if !sameMaintenanceFact(a, current) {
			return ctx, errcode.ErrForbidden
		}
	}
	return context.WithValue(ctx, maintenanceExecutionKey{}, maintenanceExecution{policy: p, admission: *a}), nil
}

type taskDefinition struct {
	JobID        string          `json:"job_id"`
	ActionID     string          `json:"action_id"`
	TriggerType  string          `json:"trigger_type"`
	CronSpec     string          `json:"cron_spec"`
	CallbackURL  string          `json:"callback_url"`
	Params       json.RawMessage `json:"params"`
	Dept         string          `json:"dept"`
	Enabled      bool            `json:"enabled"`
	TimeoutSecs  int             `json:"timeout_secs"`
	Description  string          `json:"description"`
	CreatedBy    string          `json:"created_by"`
	OwnerService string          `json:"owner_service"`
}

func readTaskDefinition(raw json.RawMessage, id string) (taskDefinition, error) {
	var d taskDefinition
	if err := json.Unmarshal(raw, &d); err != nil || d.JobID != id || d.ActionID == "" {
		return d, errcode.ErrServiceUnavailable
	}
	return d, nil
}

// MaintenanceDefinitionDigest covers reviewed definition fields, excluding only
// created/updated timestamps and next_run, which cron updates as runtime state.
func MaintenanceDefinitionDigest(raw json.RawMessage) (string, error) {
	var d taskDefinition
	if err := json.Unmarshal(raw, &d); err != nil || d.JobID == "" || d.ActionID != maintenanceAction {
		return "", errcode.ErrInvalidParams
	}
	parsed, err := decodeArchiveParams(d.Params)
	if err != nil {
		return "", err
	}
	d.Params, err = json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	canonical, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
func (p *TaskActionPolicy) authorizeDefinition(ctx context.Context, d taskDefinition) error {
	if d.ActionID != maintenanceAction {
		return nil
	}
	if err := p.authorizeAction(ctx, d.ActionID, d.Params); err != nil {
		return err
	}
	if d.TriggerType == "cron" && d.Enabled {
		if p == nil {
			return errcode.ErrForbidden
		}
		f, ok := p.fixed[d.JobID]
		if !ok || !f.active(time.Now()) {
			return errcode.ErrForbidden
		}
		raw, _ := json.Marshal(d)
		digest, err := MaintenanceDefinitionDigest(raw)
		if err != nil || digest != f.Digest {
			return errcode.ErrForbidden
		}
	}
	return nil
}
func (p *TaskActionPolicy) fixedFact(ctx context.Context, id string, params json.RawMessage) (*repository.MaintenanceAdmission, error) {
	if p == nil || p.client == nil || len(p.fixed) == 0 {
		return nil, errcode.ErrForbidden
	}
	raw, err := p.client.GetTask(ctx, id)
	if err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	var task struct {
		TaskID      string    `json:"task_id"`
		JobID       string    `json:"job_id"`
		Action      string    `json:"action"`
		Params      string    `json:"params"`
		RequestID   string    `json:"request_id"`
		SubmittedBy string    `json:"submitted_by"`
		EnqueuedAt  time.Time `json:"enqueued_at"`
	}
	if json.Unmarshal(raw, &task) != nil || task.TaskID != id || task.Action != maintenanceAction || task.JobID == "" || task.RequestID != "" || task.SubmittedBy != "" {
		return nil, errcode.ErrForbidden
	}
	fixed, ok := p.fixed[task.JobID]
	if !ok || !fixed.active(time.Now()) || task.EnqueuedAt.Before(fixed.From) || task.EnqueuedAt.After(time.Now()) {
		return nil, errcode.ErrForbidden
	}
	equal, err := jobs.EqualParams([]byte(task.Params), params)
	if err != nil || !equal {
		return nil, errcode.ErrForbidden
	}
	jobRaw, err := p.client.GetJob(ctx, task.JobID)
	if err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	job, err := readTaskDefinition(jobRaw, task.JobID)
	if err != nil || !job.Enabled || job.TriggerType != "cron" {
		return nil, errcode.ErrForbidden
	}
	digest, err := MaintenanceDefinitionDigest(jobRaw)
	if err != nil || digest != fixed.Digest {
		return nil, errcode.ErrForbidden
	}
	equal, err = jobs.EqualParams(job.Params, params)
	if err != nil || !equal {
		return nil, errcode.ErrForbidden
	}
	parsed, err := decodeArchiveParams(params)
	if err != nil {
		return nil, err
	}
	days := parsed.RetentionDays
	if days == 0 {
		days = p.defaultDays
	}
	// Scheduled maintenance uses reviewed configured defaults, never temporary
	// human overrides. A human override must have its own prewritten fact.
	if parsed.ApprovalID != "" || days != p.defaultDays {
		return nil, errcode.ErrForbidden
	}
	return &repository.MaintenanceAdmission{TaskID: id, Action: maintenanceAction, Params: normalizedTaskParams(params), Origin: "fixed_job", JobID: task.JobID, DefinitionSHA256: digest, RetentionDays: days, Reason: fixed.Reason, ValidFrom: &fixed.From, ValidUntil: &fixed.Until}, nil
}

func (e maintenanceExecution) execute(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	p := e.policy
	if p == nil || p.db == nil {
		return errcode.ErrForbidden
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if e.admission.PrincipalUserID > 0 {
		if !p.maintainers[e.admission.PrincipalUserID] {
			return errcode.ErrForbidden
		}
		if err := p.activeUser(ctx, e.admission.PrincipalUserID, tx); err != nil {
			return err
		}
	}
	valid := func() bool {
		return e.admission.ValidFrom == nil || !time.Now().Before(*e.admission.ValidFrom) && time.Now().Before(*e.admission.ValidUntil)
	}
	if !valid() {
		return errcode.ErrForbidden
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if !valid() {
		return errcode.ErrForbidden
	}
	return tx.Commit(ctx)
}
