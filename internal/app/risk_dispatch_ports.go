package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskdispatch"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskreminder"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type riskDispatchTicketPorts struct {
	service     *ticket.Service
	repo        *repository.TicketRepo
	pool        *pgxpool.Pool
	reminder    *riskreminder.Service
	remediation bool
}

func (p riskDispatchTicketPorts) PrepareRiskDispatchCreate(ctx context.Context, request *model.CreateTicketRequest,
	actorID int64) (riskdispatch.TicketPreparation, error) {
	prepare := p.service.PrepareRiskDispatchCreate
	if p.remediation {
		prepare = p.service.PrepareRiskRemediationDispatchCreate
	}
	prepared, err := prepare(ctx, request, actorID)
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

func (p riskDispatchTicketPorts) LockRiskDispatchCreateTx(ctx context.Context, tx pgx.Tx,
	preparation riskdispatch.TicketPreparation) (riskdispatch.LockedTicketCreate, error) {
	prepared, ok := preparation.(*ticket.RiskDispatchCreatePreparation)
	if !ok || prepared == nil {
		return nil, errcode.ErrInvalidParams
	}
	locked, err := p.service.LockRiskDispatchCreateTx(ctx, tx, prepared)
	if err != nil {
		return nil, err
	}
	return locked, nil
}

func (p riskDispatchTicketPorts) CreateRiskDispatchTicketTx(ctx context.Context, tx pgx.Tx,
	locked riskdispatch.LockedTicketCreate, ownerID int64, link riskdispatch.RiskDispatchLinkWriter) (riskdispatch.TicketOutcome, error) {
	prepared, ok := locked.(*ticket.LockedRiskDispatchCreate)
	if !ok || prepared == nil {
		return riskdispatch.TicketOutcome{}, errcode.ErrInvalidParams
	}
	created, err := p.service.CreateRiskDispatchTicketTx(ctx, tx, prepared, ownerID, ticket.RiskDispatchLinkWriter(link))
	if err != nil {
		return riskdispatch.TicketOutcome{}, err
	}
	if p.reminder != nil {
		if _, _, err = p.reminder.RecordConfiguredAssignment(ctx, tx, created.ID, ownerID, prepared.ActorID(), time.Now()); err != nil {
			return riskdispatch.TicketOutcome{}, err
		}
	}
	return riskdispatch.TicketOutcome{ID: created.ID, Ref: created.HumanKey}, nil
}

func (p riskDispatchTicketPorts) CanViewTicket(ctx context.Context, actorID, ticketID int64) bool {
	return p.service.CanReadRiskDispatchTicket(ctx, actorID, ticketID)
}

func (p riskDispatchTicketPorts) UpdateRiskDispatchPriorityTx(ctx context.Context, tx pgx.Tx, ticketID, actorUserID int64, priority int) error {
	return p.service.UpdateRiskDispatchPriorityTx(ctx, tx, ticketID, actorUserID, priority)
}

func (p riskDispatchTicketPorts) CurrentPublished(ctx context.Context, db riskdispatch.Reader, typeCode string) (riskdispatch.DefinitionRef, error) {
	var version *int64
	var hash *string
	var format *int
	var raw json.RawMessage
	// One statement resolves the current pointer and immutable content on the
	// caller's IAM transaction. A pool read cannot escape its failure boundary.
	err := db.QueryRow(ctx, `SELECT t.current_published_version,d.definition_hash,d.schema_format_version,d.snapshot
		FROM ticket_types t LEFT JOIN ticket_type_definitions d
		ON d.type_code=t.code AND d.definition_version=t.current_published_version
		WHERE t.code=$1 AND t.is_active`, typeCode).Scan(&version, &hash, &format, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return riskdispatch.DefinitionRef{}, errcode.ErrTicketTypeNotFound
	}
	if err != nil {
		return riskdispatch.DefinitionRef{}, err
	}
	if version == nil || hash == nil || format == nil || *format != 1 {
		return riskdispatch.DefinitionRef{}, errcode.ErrServiceUnavailable
	}
	snapshot, err := repository.DecodeBoundTicketDefinition(typeCode, raw)
	if err != nil {
		return riskdispatch.DefinitionRef{}, err
	}
	if p.remediation {
		if err := ticket.ValidateRiskDispatchDefinition(&snapshot.Type, snapshot.Fields); err != nil {
			return riskdispatch.DefinitionRef{}, err
		}
	}
	return riskdispatch.DefinitionRef{Version: *version, Hash: *hash}, nil
}

type riskDispatchOwnerDirectory struct{}

func (riskDispatchOwnerDirectory) LockOwnerFact(ctx context.Context, tx pgx.Tx, userID int64) (riskdispatch.OwnerFact, error) {
	fact := riskdispatch.OwnerFact{UserID: userID}
	err := tx.QueryRow(ctx, `
		SELECT users.status = 1 AND users.deleted_at IS NULL AND NOT COALESCE(users.is_system, false),
		       COALESCE((
		           SELECT membership.org_id
		           FROM user_orgs membership
		           JOIN organizations org ON org.id = membership.org_id
		           WHERE membership.user_id = users.id AND membership.is_primary
		             AND (membership.expires_at IS NULL OR membership.expires_at > CURRENT_TIMESTAMP)
		             AND org.status = 1 AND org.deleted_at IS NULL
		       ), 0)
		FROM users WHERE id = $1 FOR SHARE
	`, userID).Scan(&fact.Employed, &fact.PrimaryOrgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fact, nil
	}
	return fact, err
}

func (p riskDispatchTicketPorts) VisibleTicketsWhere(ctx context.Context, actorID int64) (string, []any, bool, error) {
	return p.service.RiskDispatchVisibleTicketsWhere(ctx, actorID)
}

type riskDispatchDeptRead struct {
	pool    *pgxpool.Pool
	service *riskdispatch.DeptStats
}

func (p riskDispatchDeptRead) ReadDepartmentStats(ctx context.Context, actorID int64) (riskdispatch.DepartmentStatsResult, error) {
	var result riskdispatch.DepartmentStatsResult
	err := repository.WithIAMCommandGuard(ctx, p.pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY"); err != nil {
			return err
		}
		var err error
		result.Current, err = p.service.CurrentTodoByDept(ctx, tx, actorID)
		if err != nil {
			return err
		}
		result.History, err = p.service.HistoryByDispatchDept(ctx, tx, actorID)
		return err
	})
	return result, err
}
