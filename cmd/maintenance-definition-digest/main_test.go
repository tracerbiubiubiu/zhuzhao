package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

const reviewedDefinition = `{
  "job_id": "reviewed-audit-archive",
  "action_id": "audit_archive",
  "trigger_type": "cron",
  "cron_spec": "@daily",
  "callback_url": "https://example.invalid/callback",
  "params": {"retention_days": 180},
  "dept": "operations",
  "enabled": true,
  "timeout_secs": 300,
  "description": "reviewed maintenance definition",
  "created_by": "reviewed-maintainer",
  "owner_service": "zhuzhao"
}`

func execute(input string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(nil, strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunReviewedDefinitionUsesServiceDigest(t *testing.T) {
	want, err := service.MaintenanceDefinitionDigest(json.RawMessage(reviewedDefinition))
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := execute(reviewedDefinition)
	if code != 0 || stdout != want+"\n" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q; want digest only", code, stdout, stderr)
	}
	if len(strings.TrimSuffix(stdout, "\n")) != 64 || strings.Trim(stdout, "0123456789abcdef\n") != "" {
		t.Fatalf("stdout must be one lowercase SHA256 line: %q", stdout)
	}
}

func TestRunDigestBindsReviewedFieldsAndIgnoresRuntimeFields(t *testing.T) {
	code, original, stderr := execute(reviewedDefinition)
	if code != 0 || stderr != "" {
		t.Fatalf("valid definition failed: code=%d stderr=%q", code, stderr)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(reviewedDefinition), &fields); err != nil {
		t.Fatal(err)
	}
	fields["created_at"] = "2026-11-01T00:00:00Z"
	fields["updated_at"] = "2026-11-17T00:00:00Z"
	fields["next_run"] = "2026-11-18T00:00:00Z"
	runtime, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if code, output, stderr := execute(string(runtime)); code != 0 || output != original || stderr != "" {
		t.Fatalf("runtime fields/order changed digest: code=%d stderr=%q", code, stderr)
	}
	for name, changed := range map[string]string{
		"job ID":    strings.Replace(reviewedDefinition, "reviewed-audit-archive", "another-reviewed-job", 1),
		"retention": strings.Replace(reviewedDefinition, "180", "181", 1),
		"enabled":   strings.Replace(reviewedDefinition, `"enabled": true`, `"enabled": false`, 1),
		"callback":  strings.Replace(reviewedDefinition, "/callback", "/another-callback", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if code, output, stderr := execute(changed); code != 0 || output == original || stderr != "" {
				t.Fatalf("reviewed change must change digest: code=%d stderr=%q", code, stderr)
			}
		})
	}
}

func TestRunRejectsInvalidDefinitionsWithoutEchoingInput(t *testing.T) {
	const secret = "DO_NOT_ECHO_REVIEWED_JOB_CONTENT"
	for name, input := range map[string]string{
		"empty":            "",
		"malformed":        `{"job_id":"` + secret,
		"trailing JSON":    reviewedDefinition + `{ "private": "` + secret + `" }`,
		"wrong action":     strings.Replace(reviewedDefinition, "audit_archive", secret, 1),
		"missing job ID":   `{"action_id":"audit_archive","description":"` + secret + `"}`,
		"invalid params":   `{"job_id":"reviewed","action_id":"audit_archive","params":{"private":"` + secret + `"}}`,
		"duplicate params": `{"job_id":"reviewed","action_id":"audit_archive","params":{"retention_days":180,"retention_days":181}}`,
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := execute(input)
			if code == 0 || stdout != "" || stderr == "" || strings.Contains(stderr, secret) {
				t.Fatalf("invalid input must fail without body output: code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestRunEnforcesOneMiBInputLimit(t *testing.T) {
	input := reviewedDefinition + strings.Repeat(" ", (1<<20)-len(reviewedDefinition))
	if code, stdout, stderr := execute(input); code != 0 || stdout == "" || stderr != "" {
		t.Fatalf("exactly 1MiB must be accepted: code=%d stderr=%q", code, stderr)
	}
	if code, stdout, stderr := execute(input + " "); code == 0 || stdout != "" || stderr == "" {
		t.Fatalf("over 1MiB must fail: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

type failingReader struct{ read bool }

func (r *failingReader) Read([]byte) (int, error) {
	r.read = true
	return 0, errors.New("DO_NOT_ECHO_READER_ERROR")
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("DO_NOT_ECHO_WRITER_ERROR")
}

func TestRunIOErrorsDoNotLeakErrorDetails(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		reader := &failingReader{}
		code := run(nil, reader, &stdout, &stderr)
		if code == 0 || !reader.read || stdout.Len() != 0 || stderr.Len() == 0 || strings.Contains(stderr.String(), "DO_NOT_ECHO") {
			t.Fatalf("read failure not handled privately: code=%d stderr=%q", code, stderr.String())
		}
	})
	t.Run("write", func(t *testing.T) {
		var stderr bytes.Buffer
		code := run(nil, strings.NewReader(reviewedDefinition), failingWriter{}, &stderr)
		if code == 0 || stderr.Len() == 0 || strings.Contains(stderr.String(), "DO_NOT_ECHO") {
			t.Fatalf("write failure not handled privately: code=%d stderr=%q", code, stderr.String())
		}
	})
}

func TestRunRejectsArgumentsBeforeReadingInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	reader := &failingReader{}
	code := run([]string{"--help"}, reader, &stdout, &stderr)
	if code == 0 || reader.read || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("unexpected argument handling: code=%d read=%v stdout=%q stderr=%q", code, reader.read, stdout.String(), stderr.String())
	}
}

func TestRunNeedsNoConfigurationOrServices(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("TASK_MAINTAINER_USER_IDS", "invalid-maintainer-id")
	t.Setenv("TASK_FIXED_MAINTENANCE_JOBS", "not-json")
	t.Setenv("DB_HOST", "invalid.invalid")
	t.Setenv("REDIS_ADDR", "invalid.invalid:1")
	t.Setenv("APP_ENV", "missing-config")
	code, stdout, stderr := execute(reviewedDefinition)
	if code != 0 || stdout == "" || stderr != "" {
		t.Fatalf("offline digest depends on configuration/services: code=%d stderr=%q", code, stderr)
	}
}

var _ io.Reader = (*failingReader)(nil)
var _ io.Writer = failingWriter{}
