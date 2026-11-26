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

func runDepartmentOwners(ctx context.Context, args []string, databaseURL string, input io.Reader, stdout, stderr io.Writer) int {
	resolve := args[0] == "resolve-department-owners"
	if (resolve && len(args) != 1) || (!resolve && len(args) != 3) {
		_, _ = fmt.Fprintln(stderr, "usage: resolve-department-owners < event.json | department-owner-receipt <scope_ref> <operation_id>")
		return 2
	}
	var event directorymock.DepartmentOwnerResolution
	if resolve {
		var err error
		event, err = directorymock.ReadDepartmentOwnerResolution(input)
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

	var receipt directorymock.DepartmentOwnerReceipt
	if resolve {
		receipt, err = directorymock.ResolveDepartmentOwners(ctx, pool, event)
	} else {
		receipt, err = directorymock.LookupDepartmentOwnerReceipt(ctx, pool, args[1], args[2])
	}
	if err != nil {
		if errors.Is(err, directorymock.ErrInvalidDepartmentOwners) || errors.Is(err, directorymock.ErrDepartmentOwnersConflict) ||
			errors.Is(err, directorymock.ErrDepartmentOwnersReceiptAbsent) {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		_, _ = fmt.Fprintln(stderr, "mock_department_owner_unconfirmed_query_receipt")
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
		_, _ = fmt.Fprintln(stderr, "mock_department_owner_receipt_output_failed")
		return 1
	}
	if resolve && receipt.Status != directorymock.StatusDepartmentOwnersResolved {
		return 3
	}
	return 0
}
