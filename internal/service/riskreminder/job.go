package riskreminder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// JobName is independent of the generic ticket timeout scanner and its observe flag.
const JobName = "risk_accept_timeout"

// Job is an app-mountable ReentrantJob. A tick uses one clock for both phases.
// It owns no goroutine: the app must both register AND run its CronScheduler.
type Job struct {
	service                 *Service
	now                     func() time.Time
	scanLimit, deliverLimit int
}

func NewJob(s *Service, now func() time.Time, scanLimit, deliverLimit int) *Job {
	if now == nil {
		now = time.Now
	}
	if scanLimit <= 0 {
		scanLimit = DefaultScanLimit
	}
	if deliverLimit <= 0 {
		deliverLimit = DefaultDeliverLimit
	}
	return &Job{service: s, now: now, scanLimit: scanLimit, deliverLimit: deliverLimit}
}

// NewRuntime is the production assembly seam. No approved policy source exists
// in the generic node SLA schema: absent resolver is explicitly unavailable.
// The resolver must read an approved publication on the supplied transaction;
// a test's synthetic policy is not production authorization.
func NewRuntime(db *pgxpool.Pool, inbox *repository.NotificationInboxRepo, modelText string, policy PolicyResolver, now func() time.Time, logger *slog.Logger) (*Service, *Job, error) {
	if db == nil || inbox == nil || strings.TrimSpace(modelText) == "" || policy == nil {
		return nil, nil, ErrPolicyUnavailable
	}
	s := NewService(db, inbox, NewEligibilityChecker(db, modelText, policy), logger)
	return s, NewJob(s, now, DefaultScanLimit, DefaultDeliverLimit), nil
}

// RunReport keeps generated/suppressed scan facts separate from delivery facts.
type RunReport struct {
	Scan     *ScanReport
	Delivery *DeliveryReport
}

// RunOnce is also useful for controlled-clock acceptance and recovery. Per-item
// delivery failures must reach the scheduler; a successful DB read is not a
// successful cron tick when an intent remains failed or indeterminate.
func (j *Job) RunOnce(ctx context.Context, asOf time.Time) (*RunReport, error) {
	if j == nil || j.service == nil || j.service.checker == nil {
		return nil, errors.New("riskreminder: eligibility checker not wired")
	}
	if asOf.IsZero() {
		return nil, errors.New("riskreminder: scan clock is required")
	}
	report := &RunReport{}
	var scanErr, errorDelivery error
	report.Scan, scanErr = j.service.GenerateEligibleDue(ctx, asOf, j.scanLimit)
	// Existing intents must still recover even when a different candidate's
	// configuration is unavailable. Each send independently checks eligibility.
	report.Delivery, errorDelivery = j.service.DeliverDue(ctx, asOf, j.deliverLimit)
	errs := []error{scanErr, errorDelivery}
	if report.Delivery != nil {
		for _, failure := range report.Delivery.Failed {
			errs = append(errs, fmt.Errorf("riskreminder intent %d (indeterminate=%t): %w", failure.IntentID, failure.Indeterminate, failure.Err))
		}
	}
	return report, errors.Join(errs...)
}

// Handle accepts no runtime policy/clock overrides from a task payload.
func (j *Job) Handle(ctx context.Context, params json.RawMessage) error {
	p := bytes.TrimSpace(params)
	if len(p) > 0 && !bytes.Equal(p, []byte("null")) && !bytes.Equal(p, []byte("{}")) {
		return errors.New("riskreminder: job accepts no parameters")
	}
	if j == nil {
		return errors.New("riskreminder: job not wired")
	}
	_, err := j.RunOnce(ctx, j.now())
	return err
}
