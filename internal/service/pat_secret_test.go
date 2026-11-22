package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type failingPATEntropy struct{ err error }

func (r failingPATEntropy) Read([]byte) (int, error) { return 0, r.err }

func TestGeneratePATSecretFailsClosedOnEntropyError(t *testing.T) {
	want := errors.New("synthetic entropy failure")
	secret, err := generateSecret(failingPATEntropy{err: want})
	if secret != "" || !errors.Is(err, want) {
		t.Fatalf("expected no secret and the entropy error, got secret=%t err=%v", secret != "", err)
	}
}

func TestCreatePATStopsBeforePersistenceOnEntropyError(t *testing.T) {
	service := NewPatService(nil, slog.New(slog.DiscardHandler))
	service.entropy = failingPATEntropy{err: errors.New("synthetic entropy failure")}
	row, secret, err := service.Create(context.Background(), 41, "fixture PAT", 1)
	if row != nil || secret != "" || !errors.Is(err, errcode.ErrInternal) {
		t.Fatalf("expected an internal error before persistence, got row=%t secret=%t err=%v", row != nil, secret != "", err)
	}
}
