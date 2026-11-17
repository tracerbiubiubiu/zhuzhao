package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublicEndpointSignsPublicHostWithoutChangingInternalAccess(t *testing.T) {
	var heads int
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodHead, r.Method)
		require.Equal(t, "/attachments/", r.URL.Path)
		heads++
		w.WriteHeader(http.StatusOK)
	}))
	defer internal.Close()
	cfg := &StorageConfig{Enabled: true, Endpoint: internal.URL, PublicEndpoint: "https://objects.example.invalid:9443", Bucket: "attachments", Region: "us-east-1", AccessKey: "synthetic-ak", SecretKey: "synthetic-secret"}
	cfg.ApplyDefaults()
	cli, err := NewMinioClient(cfg)
	require.NoError(t, err)
	put, err := cli.PresignPut(context.Background(), "avatars/a.png", "image/png", time.Minute)
	require.NoError(t, err)
	u, err := url.Parse(put.URL)
	require.NoError(t, err)
	require.Equal(t, "https", u.Scheme)
	require.Equal(t, "objects.example.invalid:9443", u.Host, "browser URL must use the public signing client, not Docker host")
	require.Contains(t, u.Query().Get("X-Amz-Credential"), "/us-east-1/s3/aws4_request")
	require.Contains(t, u.Query().Get("X-Amz-SignedHeaders"), "host")
	require.Equal(t, 1, heads, "bucket checks still use the internal client")
	get, err := cli.PresignGet(context.Background(), "avatars/a.png", time.Minute)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(get, "https://objects.example.invalid:9443/"))
}

func TestPublicEndpointEmptyKeepsExistingSingleEndpoint(t *testing.T) {
	cfg := &StorageConfig{Enabled: true, Endpoint: "127.0.0.1:9000", Bucket: "attachments", Region: "us-east-1", AccessKey: "synthetic-ak", SecretKey: "synthetic-secret"}
	cli, err := NewMinioClient(cfg)
	require.NoError(t, err)
	signed, err := cli.PresignGet(context.Background(), "a.txt", time.Minute)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(signed, "http://127.0.0.1:9000/"))
}

func TestPublicEndpointRejectsAmbiguousOrCredentialBearingAddresses(t *testing.T) {
	for _, endpoint := range []string{"objects.example.invalid", "ftp://objects.example.invalid", "https://secret:password@objects.example.invalid", "https://objects.example.invalid/prefix", "https://objects.example.invalid?q=x", "https://objects.example.invalid#fragment"} {
		t.Run(endpoint, func(t *testing.T) {
			cfg := &StorageConfig{Enabled: true, Endpoint: "127.0.0.1:9000", PublicEndpoint: endpoint, Bucket: "attachments", Region: "us-east-1", AccessKey: "synthetic-ak", SecretKey: "synthetic-secret"}
			cfg.ApplyDefaults()
			require.Error(t, cfg.Validate())
			_, err := NewMinioClient(cfg)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "password", "configuration errors must not echo credentials")
		})
	}
}
