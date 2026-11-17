package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestReleaseComposeStorageAndSecretBoundaries(t *testing.T) {
	raw, err := os.ReadFile("../../deployments/docker-compose.yaml")
	require.NoError(t, err)
	var doc struct {
		Services map[string]struct {
			Profiles    []string          `yaml:"profiles"`
			Environment map[string]string `yaml:"environment"`
			Ports       []string          `yaml:"ports"`
			DependsOn   yaml.Node         `yaml:"depends_on"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	app := doc.Services["app"]
	for _, key := range []string{"JWT_SECRET", "GATEWAY_SK", "TASKRUNNER_SK", "INTERNAL_JOBS_SK", "STORAGE_SECRET_KEY", "STORAGE_PUBLIC_ENDPOINT"} {
		require.Contains(t, app.Environment[key], ":?", "release must fail closed when %s is absent", key)
	}
	require.Equal(t, "true", app.Environment["STORAGE_ENABLED"])
	require.Equal(t, "http://rustfs:9000", app.Environment["STORAGE_ENDPOINT"])
	var deps map[string]struct {
		Condition string `yaml:"condition"`
	}
	require.NoError(t, app.DependsOn.Decode(&deps))
	require.Equal(t, "service_healthy", deps["rustfs"].Condition)
	rustfs := doc.Services["rustfs"]
	require.Empty(t, rustfs.Profiles, "app storage dependency must be active during default release startup")
	for _, binding := range rustfs.Ports {
		require.Contains(t, binding, "127.0.0.1:", "S3 is loopback-bound until an approved proxy exposes it")
		require.NotContains(t, binding, ":9001", "console must not be published by default")
	}
	require.NotEmpty(t, rustfs.Ports)
	for _, service := range []string{"postgres", "redis", "rustfs", "rustfs-backup"} {
		env := doc.Services[service].Environment
		key := map[string]string{"postgres": "POSTGRES_PASSWORD", "redis": "REDIS_PASSWORD", "rustfs": "RUSTFS_ROOT_PASSWORD", "rustfs-backup": "RUSTFS_ROOT_PASSWORD"}[service]
		require.Contains(t, env[key], ":?", "default public secrets must never survive release compose interpolation")
	}
}

func TestReleaseRejectsMissingOrRepoKnownDataSecrets(t *testing.T) {
	valid := func() *Config {
		return &Config{
			Server:   ServerConfig{Mode: "release", Port: 33333},
			Database: DatabaseConfig{Host: "db", Port: 5432, DBName: "zhuzhao", Password: "synthetic-database-secret"},
			Redis:    RedisConfig{Host: "redis", Password: "synthetic-redis-secret"},
			JWT:      JWTConfig{Secret: strings.Repeat("synthetic", 5), AccessTTL: time.Minute, RefreshTTL: time.Hour},
		}
	}
	require.NoError(t, valid().Validate())
	for _, value := range []string{"", "zhuzhao_dev", "zhuzhao_rustfs_dev", "zhuzhao_rustfs_bk_dev"} {
		for _, field := range []string{"database", "redis"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				c := valid()
				if field == "database" {
					c.Database.Password = value
				} else {
					c.Redis.Password = value
				}
				require.Error(t, c.Validate())
			})
		}
	}
}

func TestLoadPublicStorageEndpointEnvironmentAndEmptyCompatibility(t *testing.T) {
	t.Setenv("STORAGE_PUBLIC_ENDPOINT", "https://objects.example.invalid")
	t.Setenv("STORAGE_BUCKET", "release-bucket")
	t.Setenv("STORAGE_REGION", "us-east-1")
	cfg, err := Load("../../configs/config.yaml")
	require.NoError(t, err)
	require.Equal(t, "https://objects.example.invalid", cfg.Storage.PublicEndpoint)
	require.Equal(t, "release-bucket", cfg.Storage.Bucket)
	require.Equal(t, "us-east-1", cfg.Storage.Region)
	t.Setenv("STORAGE_PUBLIC_ENDPOINT", "")
	cfg, err = Load("../../configs/config.yaml")
	require.NoError(t, err)
	require.Empty(t, cfg.Storage.PublicEndpoint)
}
