package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service/directorymock"
)

func runRetirement(ctx context.Context, args []string, databaseURL string, input io.Reader, stdout, stderr io.Writer) int {
	resolve := args[0] == "preview-department-retirement"
	if (resolve && len(args) != 1) || (!resolve && len(args) != 3) {
		_, _ = fmt.Fprintln(stderr, "usage: preview-department-retirement < event.json | department-retirement-receipt <scope_ref> <operation_id>")
		return 2
	}
	var event directorymock.RetirementPreviewInput
	if resolve {
		var err error
		event, err = directorymock.ReadRetirementPreview(input)
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

	var receipt directorymock.RetirementPreviewReceipt
	if resolve {
		receipt, err = directorymock.PreviewRetirement(ctx, pool, event)
	} else {
		receipt, err = directorymock.LookupRetirementReceipt(ctx, pool, args[1], args[2])
	}
	if err != nil {
		if errors.Is(err, directorymock.ErrInvalidRetirement) || errors.Is(err, directorymock.ErrRetirementConflict) ||
			errors.Is(err, directorymock.ErrRetirementReceiptAbsent) {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		_, _ = fmt.Fprintln(stderr, "mock_department_retirement_unconfirmed_query_receipt")
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
		_, _ = fmt.Fprintln(stderr, "mock_department_retirement_receipt_output_failed")
		return 1
	}
	if resolve && receipt.Status != directorymock.StatusRetirementPreviewed {
		return 3
	}
	return 0
}
