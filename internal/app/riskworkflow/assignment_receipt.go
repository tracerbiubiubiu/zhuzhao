package riskworkflow

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func assignmentHash(c AssignmentCommand, target int64) [32]byte {
	body, _ := json.Marshal(struct {
		Command AssignmentCommand
		Target  int64
	}{c, target})
	return sha256.Sum256(body)
}

// Current route/credential/ticket checks precede replay. Every original member
// remains protected after detachment; replay never consumes the published edge.
func (s *AssignmentService) assignmentReplay(ctx context.Context, tx pgx.Tx, c AssignmentCommand, target int64, command string) (AssignmentResult, bool, error) {
	var hash []byte
	var kind string
	err := tx.QueryRow(ctx, `SELECT request_hash,command FROM risk_operations WHERE actor_id=$1 AND operation_id=$2`, c.ActorID, c.OperationID).Scan(&hash, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return AssignmentResult{}, false, nil
	}
	if err != nil {
		return AssignmentResult{}, false, err
	}
	want := assignmentHash(c, target)
	if kind != command || string(hash) != string(want[:]) {
		return AssignmentResult{}, false, errcode.ErrConflict
	}
	var body []byte
	var out AssignmentResult
	err = tx.QueryRow(ctx, `SELECT result FROM risk_remediation_receipts WHERE actor_id=$1 AND operation_id=$2 AND ticket_id=$3`, c.ActorID, c.OperationID, c.TicketID).Scan(&body)
	if err != nil {
		return out, false, errcode.ErrConflict
	}
	if json.Unmarshal(body, &out) != nil || out.TicketID != c.TicketID || len(out.Items) == 0 {
		return AssignmentResult{}, false, errcode.ErrServiceUnavailable
	}
	ids := make([]int64, 0, len(out.Items))
	for _, i := range out.Items {
		ids = append(ids, i.RiskID)
	}
	// Original rows are locked in global order before visibility/management reads.
	rows, err := tx.Query(ctx, `SELECT id FROM risk_items WHERE id=ANY($1) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return AssignmentResult{}, false, err
	}
	if _, err = pgx.CollectRows(rows, pgx.RowTo[int64]); err != nil {
		return AssignmentResult{}, false, err
	}
	for _, i := range out.Items {
		if err = s.assignmentRiskAccess(ctx, tx, c.ActorID, i.RiskID, target > 0); err != nil {
			return AssignmentResult{}, false, err
		}
	}
	return out, true, nil
}

func saveAssignmentReceipt(ctx context.Context, tx pgx.Tx, c AssignmentCommand, target int64, command string, out AssignmentResult) error {
	hash := assignmentHash(c, target)
	body, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO risk_operations(actor_id,operation_id,request_hash,command,result_status) VALUES($1,$2,$3,$4,'completed')`, c.ActorID, c.OperationID, hash[:], command)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO risk_remediation_receipts(actor_id,operation_id,ticket_id,result) VALUES($1,$2,$3,$4)`, c.ActorID, c.OperationID, c.TicketID, body)
	return err
}
