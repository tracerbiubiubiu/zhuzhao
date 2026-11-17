// maintenance-definition-digest prints the digest of a reviewed maintenance job.
package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

const maxDefinitionBytes = 1 << 20

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		return fail(stderr, "usage: maintenance-definition-digest < reviewed-job.json")
	}
	raw, err := io.ReadAll(io.LimitReader(stdin, maxDefinitionBytes+1))
	if err != nil {
		return fail(stderr, "cannot read reviewed maintenance definition")
	}
	if len(raw) > maxDefinitionBytes {
		return fail(stderr, "reviewed maintenance definition exceeds 1MiB")
	}
	digest, err := service.MaintenanceDefinitionDigest(json.RawMessage(raw))
	if err != nil {
		return fail(stderr, "invalid reviewed maintenance definition")
	}
	line := digest + "\n"
	if n, err := io.WriteString(stdout, line); err != nil || n != len(line) {
		return fail(stderr, "cannot write maintenance definition digest")
	}
	return 0
}

func fail(stderr io.Writer, message string) int {
	_, _ = io.WriteString(stderr, message+"\n")
	return 1
}
