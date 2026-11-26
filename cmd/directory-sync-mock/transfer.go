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

func runTransfer(ctx context.Context, args []string, databaseURL string, input io.Reader, stdout, stderr io.Writer) int {
	apply := args[0] == "transfer-primary"
	if (apply && len(args) != 1) || (!apply && len(args) != 3) {
		_, _ = fmt.Fprintln(stderr, "usage: transfer-primary < event.json | transfer-receipt <scope_ref> <operation_id>")
		return 2
	}
	var event directorymock.PrimaryTransfer
	if apply {
		var err error
		event, err = directorymock.ReadPrimaryTransfer(input)
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

	var receipt directorymock.TransferReceipt
	if apply {
		receipt, err = directorymock.ApplyPrimaryTransfer(ctx, pool, event)
	} else {
		receipt, err = directorymock.LookupTransferReceipt(ctx, pool, args[1], args[2])
	}
	if err != nil {
		if errors.Is(err, directorymock.ErrInvalidTransfer) || errors.Is(err, directorymock.ErrTransferConflict) ||
			errors.Is(err, directorymock.ErrTransferReceiptAbsent) {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		_, _ = fmt.Fprintln(stderr, "mock_transfer_unconfirmed_query_receipt")
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
		_, _ = fmt.Fprintln(stderr, "mock_transfer_receipt_output_failed")
		return 1
	}
	if apply && receipt.Status != directorymock.StatusTransferApplied {
		return 3
	}
	return 0
}
