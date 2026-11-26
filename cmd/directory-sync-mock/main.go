package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/directorymock"
)

func main() {
	// A lost stdout consumer must leave a usable retry diagnostic after commit.
	signal.Ignore(syscall.SIGPIPE)
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv("D06_MOCK_DATABASE_URL"), os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, databaseURL string, input io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 0 && databaseURL != "" {
		switch args[0] {
		case "transfer-primary", "transfer-receipt":
			return runTransfer(ctx, args, databaseURL, input, stdout, stderr)
		case "resolve-manager", "manager-receipt":
			return runManager(ctx, args, databaseURL, input, stdout, stderr)
		case "disable-employee", "employee-disable-receipt":
			return runDisable(ctx, args, databaseURL, input, stdout, stderr)
		case "resolve-department-owners", "department-owner-receipt":
			return runDepartmentOwners(ctx, args, databaseURL, input, stdout, stderr)
		case "preview-department-retirement", "department-retirement-receipt":
			return runRetirement(ctx, args, databaseURL, input, stdout, stderr)
		}
	}
	if len(args) != 0 || databaseURL == "" {
		_, _ = fmt.Fprintln(stderr, "usage: set D06_MOCK_DATABASE_URL and provide one synthetic JSON batch on stdin")
		return 2
	}
	batch, err := directorymock.ReadBatch(input)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, directorymock.ErrInvalidBatch)
		return 2
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "mock_directory_database_unavailable")
		return 1
	}
	defer func() { _ = conn.Close(context.Background()) }()

	receipt, err := directorymock.Apply(ctx, conn, batch)
	if err != nil {
		if errors.Is(err, directorymock.ErrBatchConflict) || errors.Is(err, directorymock.ErrBaselineConflict) ||
			errors.Is(err, directorymock.ErrInvalidBatch) {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		_, _ = fmt.Fprintln(stderr, "mock_directory_apply_unconfirmed_retry_same_batch")
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
		_, _ = fmt.Fprintln(stderr, "mock_directory_receipt_output_failed_retry_same_batch")
		return 1
	}
	if receipt.Status != directorymock.StatusSucceeded {
		return 3
	}
	return 0
}
