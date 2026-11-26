package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/directorymock"
)

func runDisable(ctx context.Context, args []string, databaseURL string, input io.Reader, stdout, stderr io.Writer) int {
	apply := args[0] == "disable-employee"
	if (apply && len(args) != 1) || (!apply && len(args) != 3) {
		_, _ = fmt.Fprintln(stderr, "usage: disable-employee < event.json | employee-disable-receipt <scope_ref> <operation_id>")
		return 2
	}
	var event directorymock.EmployeeDisable
	if apply {
		var err error
		event, err = directorymock.ReadEmployeeDisable(input)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "mock_directory_database_unavailable")
		return 1
	}
	defer pool.Close()

	var output any
	exitCode := 0
	if apply {
		var receipt directorymock.DisableReceipt
		receipt, err = directorymock.DisableEmployee(ctx, pool, event, func(ctx context.Context, event directorymock.EmployeeDisable, runID string) (int64, string, error) {
			return service.RecordMockDirectoryDisable(ctx, pool, service.MockDirectoryTermination{
				ScopeRef: event.ScopeRef, EmployeeExternalID: event.EmployeeExternalID,
				IdentityGeneration: event.IdentityGeneration, SourceRevision: event.SourceRevision,
			}, runID)
		})
		output = receipt
		if receipt.Status != directorymock.StatusEmployeeDisabled {
			exitCode = 3
		}
	} else {
		output, err = directorymock.LookupDisableReceipt(ctx, pool, args[1], args[2])
	}
	if err != nil {
		if errors.Is(err, directorymock.ErrInvalidDisable) || errors.Is(err, directorymock.ErrDisableConflict) ||
			errors.Is(err, directorymock.ErrDisableReceiptAbsent) {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		_, _ = fmt.Fprintln(stderr, "mock_disable_unconfirmed_query_receipt")
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		_, _ = fmt.Fprintln(stderr, "mock_disable_receipt_output_failed")
		return 1
	}
	return exitCode
}
