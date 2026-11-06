//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// TestConfirm_RichtextIndependentQuota checks the full direct-upload confirm route:
// images remain usable at the attachment limit, while the next ordinary attachment is rejected.
func TestConfirm_RichtextIndependentQuota(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx := context.Background()
	ticketID := insertTicket(t, f.uid)
	confirm := func(purpose string) error {
		p, err := f.svc.PresignUpload(ctx, &model.PresignUploadInput{Purpose: purpose, FileName: "a.png", MIMEType: "image/png", SizeBytes: 10, TicketID: &ticketID}, f.uid)
		require.NoError(t, err)
		_, err = f.svc.Confirm(ctx, &model.ConfirmFileInput{ObjectKey: p.ObjectKey, TicketID: &ticketID}, f.uid)
		return err
	}
	require.NoError(t, confirm("ticket_attachment"))
	require.NoError(t, confirm("ticket_richtext"))
	require.NoError(t, confirm("ticket_richtext"))
	err := confirm("ticket_attachment")
	require.Error(t, err)
	require.Equal(t, errcode.ErrInvalidParams.Code, codeOf(t, err))
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, ticketID).Scan(&count))
	require.Equal(t, 3, count)
}
