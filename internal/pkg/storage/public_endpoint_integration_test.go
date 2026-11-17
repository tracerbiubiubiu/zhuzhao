//go:build integration

package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Two real origins prove that the public Host is signed at source rather than
// rewritten after signing. All data and resources belong to this test only.
func TestPublicEndpointRustFSUploadsThroughSignedHostAndReadsInternally(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{
		Image:        "rustfs/rustfs:1.0.5@sha256:b81d16a90ee25bd9511280127ca1471788c6e874cf0f0da759645f46b2dc1d74",
		ExposedPorts: []string{"9000/tcp"}, Env: map[string]string{"RUSTFS_ROOT_USER": "public-signing-test", "RUSTFS_ROOT_PASSWORD": "synthetic-signing-test-secret"},
		Cmd: []string{"server", "/data", "--console-address", ":9001"}, WaitingFor: wait.ForHTTP("/health").WithPort("9000/tcp"),
	}, Started: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "9000/tcp")
	require.NoError(t, err)
	internalURL := fmt.Sprintf("http://%s:%s", host, port.Port())
	target, err := url.Parse(internalURL)
	require.NoError(t, err)
	// NewSingleHostReverseProxy changes the upstream address but preserves Host,
	// exactly as the deployment's browser-facing S3 proxy must do.
	proxy := httptest.NewServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	cfg := &StorageConfig{Enabled: true, Endpoint: internalURL, PublicEndpoint: proxy.URL, Region: "us-east-1", Bucket: "public-signing-test", AccessKey: "public-signing-test", SecretKey: "synthetic-signing-test-secret"}
	cfg.ApplyDefaults()
	client, err := NewMinioClient(cfg)
	require.NoError(t, err)
	signed, err := client.PresignPut(ctx, "avatars/representative.png", "image/png", time.Minute)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, signed.URL, bytes.NewBufferString("representative-recovery-object"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "image/png")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	stat, err := client.StatObject(ctx, "avatars/representative.png")
	require.NoError(t, err)
	require.Equal(t, int64(len("representative-recovery-object")), stat.Size)
	reader, _, err := client.GetObject(ctx, "avatars/representative.png", stat.ETag)
	require.NoError(t, err)
	restored, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, "representative-recovery-object", string(restored))
	// Bypassing the public proxy changes Host and must invalidate the signature.
	bad, err := url.Parse(signed.URL)
	require.NoError(t, err)
	bad.Host = target.Host
	wrong, err := http.NewRequestWithContext(ctx, http.MethodPut, bad.String(), bytes.NewBufferString("wrong-host"))
	require.NoError(t, err)
	wrong.Header.Set("Content-Type", "image/png")
	rejected, err := http.DefaultClient.Do(wrong)
	require.NoError(t, err)
	_ = rejected.Body.Close()
	require.Equal(t, http.StatusForbidden, rejected.StatusCode)
	// Keep a separate backup artifact, remove the source, then restore the same
	// representative key. This is a synthetic drill, not an off-host backup claim.
	backup := filepath.Join(t.TempDir(), "representative-object.backup")
	require.NoError(t, os.WriteFile(backup, restored, 0o600))
	require.NoError(t, client.RemoveObject(ctx, "avatars/representative.png"))
	_, err = client.StatObject(ctx, "avatars/representative.png")
	require.ErrorIs(t, err, ErrObjectNotFound)
	recovery, err := os.ReadFile(backup)
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(restored), sha256.Sum256(recovery))
	recoveryURL, err := client.PresignPut(ctx, "avatars/representative.png", "image/png", time.Minute)
	require.NoError(t, err)
	recoveryReq, err := http.NewRequestWithContext(ctx, http.MethodPut, recoveryURL.URL, bytes.NewReader(recovery))
	require.NoError(t, err)
	recoveryReq.Header.Set("Content-Type", "image/png")
	recoveryResp, err := http.DefaultClient.Do(recoveryReq)
	require.NoError(t, err)
	_ = recoveryResp.Body.Close()
	require.Equal(t, http.StatusOK, recoveryResp.StatusCode)
	recoveredStat, err := client.StatObject(ctx, "avatars/representative.png")
	require.NoError(t, err)
	require.Equal(t, stat.ETag, recoveredStat.ETag)
	require.NoError(t, client.RemoveObject(ctx, "avatars/representative.png"))
}
