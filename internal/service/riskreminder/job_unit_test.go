package riskreminder_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskreminder"
)

var _ service.ReentrantJob = (*riskreminder.Job)(nil)

func TestRuntimeHasNoImplicitProductionPolicy(t *testing.T) {
	s, job, err := riskreminder.NewRuntime(nil, nil, "", nil, nil, nil)
	if !errors.Is(err, riskreminder.ErrPolicyUnavailable) || s != nil || job != nil {
		t.Fatalf("missing policy must close runtime: service=%v job=%v error=%v", s, job, err)
	}
}

func TestJobRejectsUnwiredAndPayloadPolicyOverride(t *testing.T) {
	job := riskreminder.NewJob(nil, nil, 0, 0)
	if err := job.Handle(context.Background(), nil); err == nil {
		t.Fatal("unwired job reported success")
	}
	if err := job.Handle(context.Background(), []byte(`{"accept_minutes":30}`)); err == nil {
		t.Fatal("task payload supplied policy")
	}
}
