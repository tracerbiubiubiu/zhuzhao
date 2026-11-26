package repository

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

const (
	IAMUserRoles = "iam.user.roles.set"
	IAMRoleMenus = "iam.role.menus.set"
)

type IAMOperation struct {
	ID            string
	Command       string
	Actor, Target int64
	InputHash     []byte
}

func FindIAMReceipt(ctx context.Context, db IAMDBTX, actor int64, id string) (*model.IAMReceipt, []byte, error) {
	r := model.IAMReceipt{OperationID: id, Status: "domain_committed"}
	var hash []byte
	err := db.QueryRow(ctx, `SELECT r.command,r.target_id,r.input_hash,r.record_id IS NOT NULL,COALESCE(f.policy_changed,false)
 FROM iam_command_receipts r LEFT JOIN iam_audit_facts f USING(record_id)
 WHERE r.actor_id=$1 AND r.operation_id=$2`, actor, id).Scan(&r.Command, &r.Target, &hash, &r.Changed, &r.PolicyChanged)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &r, hash, nil
}

func (op IAMOperation) Replayed(ctx context.Context, db IAMDBTX) (bool, error) {
	r, hash, err := FindIAMReceipt(ctx, db, op.Actor, op.ID)
	if err != nil || r == nil {
		return false, err
	}
	if r.Command != op.Command || r.Target != op.Target || !bytes.Equal(hash, op.InputHash) {
		return false, errcode.ErrConcurrentModification
	}
	return true, nil
}

func (r *RoleRepo) MenuOperationReplayed(ctx context.Context, op IAMOperation) (bool, error) {
	return op.Replayed(ctx, IAMDB(ctx, r.db))
}

// RecordMenuAudit appends to the transaction that owns the menu replacement.
func (r *RoleRepo) RecordMenuAudit(ctx context.Context, op IAMOperation, previous, current []int64, policyChanged bool, collector string) error {
	state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState)
	if !ok || state.shared || state.pool != r.db {
		return errors.New("IAM menu audit requires the owning write transaction")
	}
	return op.Record(ctx, state.tx, previous, current, policyChanged, collector)
}

func (op IAMOperation) Record(ctx context.Context, tx pgx.Tx, previous, current []int64, policyChanged bool, collector string) error {
	var recordID *string
	if policyChanged || !sameMenuSet(previous, current) {
		var id string
		err := tx.QueryRow(ctx, `
    INSERT INTO iam_audit_facts(event_name, actor_id, target_id, added_ids, removed_ids,policy_changed)
    VALUES ($1,$2,$3, ARRAY(SELECT unnest($5::bigint[]) EXCEPT SELECT unnest($4::bigint[]) ORDER BY 1),
      ARRAY(SELECT unnest($4::bigint[]) EXCEPT SELECT unnest($5::bigint[]) ORDER BY 1),$6)
    RETURNING record_id`, op.Command, op.Actor, op.Target, previous, current, policyChanged).Scan(&id)
		if err != nil {
			return err
		}
		recordID = &id
		if collector != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO iam_audit_deliveries(record_id,collector_key) VALUES($1,$2)`, id, collector); err != nil {
				return err
			}
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO iam_command_receipts(actor_id,operation_id,command,target_id,input_hash,record_id)
 VALUES($1,$2,$3,$4,$5,$6)`, op.Actor, op.ID, op.Command, op.Target, op.InputHash, recordID)
	return err
}

// MenuPolicyChanged compares the concrete Casbin projection to the menu-derived proposal.
// Any legacy extra policy field disappears on rebuild, including an otherwise identical route.
func (r *RoleRepo) MenuPolicyChanged(ctx context.Context, role *model.Role, apis []*model.MenuAPI) (bool, error) {
	if role.Code == "admin" || role.Code == "superadmin" {
		return false, nil
	}
	rows, err := IAMDB(ctx, r.db).Query(ctx, `SELECT COALESCE(v1,''),COALESCE(v2,''),
    COALESCE(v3,'')<>'' OR COALESCE(v4,'')<>'' OR COALESCE(v5,'')<>'' FROM casbin_rule WHERE ptype='p' AND v0=$1`, "role::"+role.Code)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	expected := make(map[[2]string]struct{}, len(apis))
	for _, api := range apis {
		expected[[2]string{api.APIPath, api.APIMethod}] = struct{}{}
	}
	changed := false
	for rows.Next() {
		var path, method string
		var extra bool
		if err := rows.Scan(&path, &method, &extra); err != nil {
			return false, err
		}
		key := [2]string{path, method}
		_, found := expected[key]
		changed = changed || !found || extra
		delete(expected, key)
	}
	return changed || len(expected) > 0, rows.Err()
}
