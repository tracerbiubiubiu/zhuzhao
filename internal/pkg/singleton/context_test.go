package singleton

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDetachUsesOwnerLifetimeAndRequestValues(t *testing.T) {
	type valueKey struct{}
	ownerDeadline := time.Now().Add(time.Minute)
	owner, loseOwner := context.WithDeadline(context.Background(), ownerDeadline)
	defer loseOwner()
	request, cancelRequest := context.WithTimeout(context.WithValue(context.Background(), valueKey{}, "request"), time.Second)
	defer cancelRequest()

	detached := Detach(WithOwner(request, owner))
	// Write paths also apply a per-operation timeout over the detached context.
	write, cancelWrite := context.WithTimeout(detached, time.Second)
	defer cancelWrite()
	cancelRequest()
	if err := write.Err(); err != nil {
		t.Fatalf("client cancellation stopped the write: %v", err)
	}
	if got := write.Value(valueKey{}); got != "request" {
		t.Fatalf("request value = %v, want request", got)
	}
	if got, ok := detached.Deadline(); !ok || !got.Equal(ownerDeadline) {
		t.Fatalf("detached deadline = %v (%t), want %v", got, ok, ownerDeadline)
	}
	loseOwner()
	select {
	case <-write.Done():
		if !errors.Is(write.Err(), context.Canceled) {
			t.Fatalf("owner cancellation error = %v", write.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("owner cancellation did not stop the detached write")
	}
}

func TestDetachWithoutOwnerKeepsLegacyBehavior(t *testing.T) {
	request, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel()
	detached := Detach(request)
	if detached.Done() != nil || detached.Err() != nil {
		t.Fatal("detaching an unbound request retained its cancellation")
	}
	if _, ok := detached.Deadline(); ok {
		t.Fatal("detaching an unbound request retained its deadline")
	}
}
