package singleton

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDrainClosesAdmissionBeforeWaitingForAcceptedRequest(t *testing.T) {
	g := &Guard{}
	g.active.Store(true)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := g.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { close(entered); <-release }))
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/write", nil))
		close(finished)
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, g.DrainRequests(ctx), context.DeadlineExceeded, "drain cannot pretend a live handler has exited")
	after := httptest.NewRecorder()
	handler.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	require.Equal(t, http.StatusServiceUnavailable, after.Code)
	close(release)
	<-finished
	require.NoError(t, g.DrainRequests(context.Background()))
}
