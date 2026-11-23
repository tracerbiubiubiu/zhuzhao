package repository

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// RiskTicketLinkBinding is an ordered identity snapshot, not authorization.
// OwnerAtLink/OwnerUserID preserve the original dispatch/group owner, not the
// current risk reviewer or ticket handler. The risk owner must lock/revalidate
// CURRENT risk responsibility and permissions before ticket locks.
type RiskTicketLinkBinding struct {
	LinkID            int64  `json:"link_id"`
	RiskID            int64  `json:"risk_id"`
	GroupID           int64  `json:"group_id"`
	Revision          int64  `json:"revision"`
	OwnerAtLink       int64  `json:"owner_at_link"`
	OwnerUserID       int64  `json:"owner_user_id"`
	TargetOrgID       int64  `json:"target_org_id"`
	TypeCode          string `json:"type_code"`
	DefinitionVersion int64  `json:"definition_version"`
	GroupActive       bool   `json:"group_active"`
}

// ReadActiveRiskTicketBindingTx never acquires risk/group/link row locks. After
// locking a ticket it is safe only as a binding check; it cannot admit risk work.
// All writers adding active links to EXISTING tickets must lock their risk scope
// first, then the ticket, then write the link, and retain locks through commit.
func (r *TicketRepo) ReadActiveRiskTicketBindingTx(ctx context.Context, tx pgx.Tx, ticketID int64) ([]RiskTicketLinkBinding, error) {
	var links, groups bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('risk_ticket_links') IS NOT NULL,
  to_regclass('risk_dispatch_groups') IS NOT NULL`).Scan(&links, &groups); err != nil {
		return nil, err
	}
	if !links && !groups {
		return nil, nil
	}
	if !links || !groups {
		return nil, errcode.ErrServiceUnavailable
	}
	var raw json.RawMessage
	err := tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object(
  'link_id',l.id,'risk_id',l.risk_id,'group_id',l.group_id,'revision',l.revision,
  'owner_at_link',l.owner_at_link,'owner_user_id',g.owner_user_id,
  'target_org_id',g.target_org_id,'type_code',g.type_code,
  'definition_version',g.definition_version,'group_active',g.ended_at IS NULL)
  ORDER BY l.id),'[]'::jsonb)
  FROM risk_ticket_links l JOIN risk_dispatch_groups g ON g.id=l.group_id
  WHERE g.ticket_id=$1 AND l.ended_at IS NULL`, ticketID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var result []RiskTicketLinkBinding
	if json.Unmarshal(raw, &result) != nil || result == nil {
		return nil, errcode.ErrServiceUnavailable
	}
	return result, nil
}
