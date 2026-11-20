//go:build integration

package ticket

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestMessageDispatcherPreferenceAndLedgerFailuresAreVisible(t *testing.T) {
	ctx := context.Background()
	biz := "nf_" + uniqueSuffix()
	tpl := "fixture_" + uniqueSuffix()
	_, err := testPool.Exec(ctx, `
		INSERT INTO message_templates(code,name,channel,subject,body,enabled)
		VALUES($1,'fixture','uip','fixture subject','fixture body',true)`, tpl)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM message_log WHERE biz_type=$1`, biz)
		_, _ = testPool.Exec(ctx, `DELETE FROM message_rules WHERE biz_type=$1`, biz)
		_, _ = testPool.Exec(ctx, `DELETE FROM message_templates WHERE code=$1`, tpl)
	})
	_, err = testPool.Exec(ctx, `
		INSERT INTO message_rules (biz_type,event_action,template_code,recipient_type,fixed_recipients,enabled)
		VALUES($1,$2,$3,'fixed','1',true)`, biz, domainevent.TicketCreated, tpl)
	require.NoError(t, err)
	closed, err := pgxpool.NewWithConfig(ctx, testPool.Config())
	require.NoError(t, err)
	closed.Close()
	for _, tc := range []struct {
		name, expected  string
		optouts, ledger *pgxpool.Pool
		sends           int
	}{
		{"preference unreadable", "notify: recipient preference lookup failed", closed, testPool, 0},
		{"delivery ledger unwritable", "notify: delivery record write failed", testPool, closed, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			sender := &captureSender{}
			dispatcher := NewMessageDispatcher(repository.NewMessageTemplateRepo(testPool),
				repository.NewMessageRuleRepo(testPool), repository.NewMessageLogRepo(tc.ledger),
				repository.NewNotificationOptoutRepo(tc.optouts), sender, slog.New(slog.NewTextHandler(&logs, nil)), nil)
			dispatcher.Dispatch(ctx, domainevent.DomainEvent{BizType: domainevent.BizType(biz), Action: domainevent.TicketCreated, BizID: "1"})
			require.Len(t, sender.messages(), tc.sends)
			require.Contains(t, logs.String(), tc.expected)
		})
	}
}
