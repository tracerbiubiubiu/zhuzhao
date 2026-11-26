package directorymock

import (
	"errors"
	"strings"
	"testing"
)

const sampleBatch = `{"provider_ref":"mock-directory","domain":"departments","mode":"full",
"scope_ref":"synthetic-demo","batch_id":"batch-1","success_cursor":"snapshot-1",
"pages":[{"cursor":"","done":true,"records":[
{"external_id":"dept-a","display_name":"合成部门甲","source_revision":1}]}]}`

func TestReadBatchRejectsAmbiguousOrOutOfScopeInput(t *testing.T) {
	cases := map[string]string{
		"other provider":   strings.Replace(sampleBatch, "mock-directory", "real-directory", 1),
		"other domain":     strings.Replace(sampleBatch, "departments", "users", 1),
		"other mode":       strings.Replace(sampleBatch, "full", "incremental", 1),
		"duplicate member": strings.Replace(sampleBatch, `"mode":"full"`, `"mode":"full","mode":"full"`, 1),
		"unknown member":   strings.Replace(sampleBatch, `"done":true`, `"done":true,"complete":true`, 1),
		"trailing value":   sampleBatch + ` {}`,
		"oversize":         sampleBatch + strings.Repeat(" ", maxBatchBytes-len(sampleBatch)+1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadBatch(strings.NewReader(input)); !errors.Is(err, ErrInvalidBatch) {
				t.Fatalf("ReadBatch error = %v, want ErrInvalidBatch", err)
			}
		})
	}
}

func TestReadBatchAcceptsExactLimitAndChineseDepartmentName(t *testing.T) {
	input := sampleBatch + strings.Repeat(" ", maxBatchBytes-len(sampleBatch))
	batch, err := ReadBatch(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if record := batch.Pages[0].Records[0]; !record.valid() || record.DisplayName != "合成部门甲" {
		t.Fatalf("unexpected department %+v", record)
	}
}
