package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IAMAuditDelivery struct {
	pool          *pgxpool.Pool
	endpoint, key string
	client        *http.Client
}

func NewIAMAuditDelivery(pool *pgxpool.Pool, endpoint string) (*IAMAuditDelivery, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback())) {
		return nil, errors.New("invalid IAM audit collector configuration")
	}
	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &IAMAuditDelivery{pool: pool, endpoint: endpoint, key: collectorKey(endpoint), client: client}, nil
}

// Handle claims one original record. available_at doubles as the lease expiry;
// a crashed worker leaves a durable, reclaimable intent and has no IAM writer.
func (w *IAMAuditDelivery) Handle(ctx context.Context, _ json.RawMessage) error {
	var id string
	var fence int64
	var payload []byte
	err := w.pool.QueryRow(ctx, `
        WITH next AS (
            SELECT record_id FROM iam_audit_deliveries
            WHERE collector_key = $1 AND delivered_at IS NULL AND available_at <= now()
            ORDER BY available_at, record_id LIMIT 1 FOR UPDATE SKIP LOCKED
        ), claim AS (
            UPDATE iam_audit_deliveries d
            SET fence = fence + 1, available_at = now() + interval '30 seconds'
            FROM next WHERE d.record_id = next.record_id
            RETURNING d.record_id, d.fence
        )
        SELECT c.record_id, c.fence, jsonb_build_object(
            'record_id', f.record_id, 'event_name', f.event_name,
            'occurred_at', to_char(f.occurred_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
            'actor_id', f.actor_id::text, 'target_id', f.target_id::text,
            'added_ids', f.added_ids::text[], 'removed_ids', f.removed_ids::text[], 'outcome', 'success'
        ) FROM claim c JOIN iam_audit_facts f USING (record_id)`, w.key).Scan(&id, &fence, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("IAM audit request construction failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", id)
	resp, err := w.client.Do(req)
	if err != nil {
		return errors.New("IAM audit delivery unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("IAM audit delivery rejected")
	}
	confirmed, err := w.pool.Exec(ctx, `UPDATE iam_audit_deliveries SET delivered_at = clock_timestamp()
        WHERE record_id = $1 AND fence = $2 AND available_at > clock_timestamp() AND delivered_at IS NULL`, id, fence)
	if err == nil && confirmed.RowsAffected() == 0 {
		return errors.New("IAM audit delivery lease expired")
	}
	return err
}
