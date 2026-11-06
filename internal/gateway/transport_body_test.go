package gateway

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

type transportBody struct {
	io.Reader
	readErr, closeErr error
	closes            int
}

func (b *transportBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.Reader.Read(p)
}

func (b *transportBody) Close() error { b.closes++; return b.closeErr }

// Exercise the pinned signing Transport over real HTTP, including its local
// pre-send failure. A failed read still owns the original body's cleanup.
func TestSigningTransport_BodyLifecycle(t *testing.T) {
	const payload = `{"value":"signed"}`
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(req.Body)
		if err != nil || string(body) != payload {
			t.Errorf("upstream body=%q err=%v", body, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := (&aksk.Verifier{Keys: map[string][]byte{"consumer": []byte("synthetic-transport-key")}}).Verify(req, body); err != nil {
			t.Errorf("upstream signature: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	client := &http.Client{Transport: &aksk.Transport{AK: "consumer", SK: []byte("synthetic-transport-key")}}
	readErr, closeErr := errors.New("synthetic read failure"), errors.New("synthetic close failure")
	for _, tc := range []struct {
		name              string
		readErr, closeErr error
	}{
		{"success", nil, nil}, {"read_error", readErr, nil},
		{"read_and_close_error", readErr, closeErr}, {"close_error_after_read", nil, closeErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &transportBody{Reader: strings.NewReader(payload), readErr: tc.readErr, closeErr: tc.closeErr}
			req, err := http.NewRequest(http.MethodPost, upstream.URL+"/ingest", body)
			require.NoError(t, err)
			before := calls.Load()
			resp, err := client.Do(req)
			if tc.readErr != nil {
				require.ErrorIs(t, err, readErr, "read error must not be replaced by Close's error")
				require.Nil(t, resp)
				require.Equal(t, before, calls.Load(), "failed body must not be sent upstream")
			} else {
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				require.Equal(t, http.StatusOK, resp.StatusCode)
				require.Equal(t, before+1, calls.Load())
			}
			require.Equal(t, 1, body.closes, "signing Transport closes its original body exactly once")
		})
	}
}
