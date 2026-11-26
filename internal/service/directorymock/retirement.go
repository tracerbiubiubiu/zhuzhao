package directorymock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	StatusRetirementPreviewed = "previewed"
	statusRetirementPending   = "pending"
	maxRetirementInputBytes   = 4096
)

var (
	ErrInvalidRetirement       = errors.New("invalid_mock_department_retirement")
	ErrRetirementConflict      = errors.New("mock_department_retirement_content_conflict")
	ErrRetirementReceiptAbsent = errors.New("mock_department_retirement_receipt_not_found")
)

type RetirementPreviewInput struct {
	ProviderRef                string `json:"provider_ref"`
	ScopeRef                   string `json:"scope_ref"`
	OperationID                string `json:"operation_id"`
	SourceDepartmentExternalID string `json:"source_department_external_id"`
	TargetDepartmentExternalID string `json:"target_department_external_id,omitempty"`
	ApprovedMappingRef         string `json:"approved_mapping_ref,omitempty"`
}

type RetirementPreviewReceipt struct {
	RetirementPreviewInput
	RunID       string          `json:"run_id"`
	Status      string          `json:"status"`
	Problem     string          `json:"problem,omitempty"`
	PreviewHash string          `json:"preview_hash"`
	Snapshot    json.RawMessage `json:"snapshot"`
}

func ReadRetirementPreview(input io.Reader) (RetirementPreviewInput, error) {
	var event RetirementPreviewInput
	reader := &io.LimitedReader{R: input, N: maxRetirementInputBytes + 1}
	if err := jsonv2.UnmarshalRead(reader, &event, jsonv2.RejectUnknownMembers(true)); err != nil ||
		reader.N == 0 || !event.valid() {
		return RetirementPreviewInput{}, ErrInvalidRetirement
	}
	return event, nil
}

func (event RetirementPreviewInput) valid() bool {
	return event.ProviderRef == repository.MockDirectorySource && validRef(event.ScopeRef) && validRef(event.OperationID) &&
		validRef(event.SourceDepartmentExternalID) &&
		(event.TargetDepartmentExternalID == "" || validRef(event.TargetDepartmentExternalID)) &&
		(event.ApprovedMappingRef == "" || validRef(event.ApprovedMappingRef))
}

func retirementOperationKey(scope, operation string) operationKey {
	return operationKey{scope: scope, id: "retirement-preview/" + operation}
}

// PreviewRetirement records a synthetic mapping observation. It never changes
// the organizations, permission sources, tickets, or history in the snapshot.
func PreviewRetirement(ctx context.Context, pool *pgxpool.Pool, event RetirementPreviewInput) (RetirementPreviewReceipt, error) {
	if !event.valid() {
		return RetirementPreviewReceipt{}, ErrInvalidRetirement
	}
	key := retirementOperationKey(event.ScopeRef, event.OperationID)
	receipt, err := recordOperation(ctx, pool, key, event, func(ctx context.Context, db repository.IAMDBTX, runID string) (RetirementPreviewReceipt, error) {
		return retirementOutcome(ctx, db, event, runID)
	})
	switch {
	case errors.Is(err, errScopeAbsent):
		err = ErrInvalidRetirement
	case errors.Is(err, errOperationConflict):
		err = ErrRetirementConflict
	}
	return receipt, err
}

func LookupRetirementReceipt(ctx context.Context, pool *pgxpool.Pool, scope, operation string) (RetirementPreviewReceipt, error) {
	if !validRef(scope) || !validRef(operation) {
		return RetirementPreviewReceipt{}, ErrInvalidRetirement
	}
	receipt, _, err := readOperationReceipt[RetirementPreviewReceipt](ctx, pool, retirementOperationKey(scope, operation))
	if errors.Is(err, errOperationAbsent) {
		err = ErrRetirementReceiptAbsent
	}
	return receipt, err
}

func retirementOutcome(ctx context.Context, db repository.IAMDBTX, event RetirementPreviewInput, runID string) (RetirementPreviewReceipt, error) {
	receipt := RetirementPreviewReceipt{
		RetirementPreviewInput: event,
		RunID:                  runID,
		Status:                 statusRetirementPending,
	}
	var sourceAvailable, mappingApproved, targetAvailable bool
	err := db.QueryRow(ctx, retirementSnapshotSQL(), event.ScopeRef, event.SourceDepartmentExternalID,
		event.TargetDepartmentExternalID, event.ApprovedMappingRef, repository.MockDirectorySource).
		Scan(&sourceAvailable, &mappingApproved, &targetAvailable, &receipt.Snapshot)
	if err != nil {
		return RetirementPreviewReceipt{}, fmt.Errorf("read mock retirement facts: %w", err)
	}
	switch {
	case !sourceAvailable:
		receipt.Problem = "source_department_unresolved"
	case event.ApprovedMappingRef == "":
		receipt.Problem = "mapping_unavailable"
	case event.TargetDepartmentExternalID == "":
		receipt.Problem = "target_department_unresolved"
	case !mappingApproved:
		receipt.Problem = "mapping_unavailable"
	case !targetAvailable:
		receipt.Problem = "target_department_unresolved"
	default:
		receipt.Status = StatusRetirementPreviewed
	}

	// Exclude generated receipt identifiers so two fresh requests can compare
	// the same facts and mapping independently of their operation IDs.
	hashInput := receipt
	hashInput.RunID = ""
	hashInput.OperationID = ""
	data, err := json.Marshal(hashInput)
	if err != nil {
		return RetirementPreviewReceipt{}, fmt.Errorf("hash mock retirement preview: %w", err)
	}
	hash := sha256.Sum256(data)
	receipt.PreviewHash = hex.EncodeToString(hash[:])
	return receipt, nil
}

// Each observation comes from one PostgreSQL snapshot. The scope of this mock
// preview is an entity and its directly attached live virtual groups.
func retirementSnapshotSQL() string {
	// The reserved mapping namespace is pre-seeded in the synthetic database.
	// A preview command cannot create or update approvals.
	return `WITH approved_mapping AS (
		SELECT run_id FROM directory_mock_transfer_runs
		WHERE scope_ref=$1 AND operation_id='retirement-mapping/' || $4 AND $4 <> ''
		  AND receipt @> jsonb_build_object('approved', true, 'source_department_external_id', $2::text,
				'target_department_external_id', $3::text)
	), bound_departments AS (
		SELECT b.external_id, d.source_revision, o.id, o.tenant_id, o.version, o.parent_id, o.path::text AS path,
		       o.source, o.ticket_visibility, o.owner_user_ids
		FROM directory_mock_department_bindings b
		JOIN directory_mock_departments d USING (scope_ref, external_id)
		JOIN organizations o ON o.id=b.org_id
		WHERE b.scope_ref=$1 AND b.external_id IN ($2,$3) AND o.source=$5
		  AND NOT o.is_virtual AND o.is_system IS FALSE AND (` + repository.OrgSourceActiveSQL("o") + `)
	), source_dept AS (
		SELECT * FROM bound_departments WHERE external_id=$2
	), target_dept AS (
		SELECT candidate.* FROM bound_departments candidate CROSS JOIN source_dept source
		JOIN organizations target_org ON target_org.id=candidate.id
		WHERE candidate.external_id=$3 AND candidate.tenant_id=source.tenant_id
		  AND EXISTS (SELECT 1 FROM approved_mapping)
		  AND source.id NOT IN (` + repository.OrgAncestorIDsSQL("target_org") + `)
	), virtual_groups AS (
		SELECT v.id, v.version, v.parent_id, v.path::text AS path, v.source, v.ticket_visibility, v.owner_user_ids
		FROM organizations v JOIN source_dept s ON v.parent_id=s.id AND v.tenant_id=s.tenant_id
		WHERE v.is_virtual AND v.is_system IS FALSE AND v.status=1 AND v.deleted_at IS NULL
	), source_orgs AS (
		SELECT id FROM source_dept UNION ALL SELECT id FROM virtual_groups
	), scope_orgs AS (
		SELECT id FROM source_orgs UNION ALL SELECT id FROM target_dept
	), in_flight_tickets AS (
		SELECT t.id, t.org_id, t.org_path::text AS org_path, t.assigned_to, t.status, t.revision
		FROM tickets t JOIN source_orgs o ON t.org_id=o.id
		WHERE t.status NOT IN ('closed','rejected')
	), in_flight_risks AS (
		SELECT r.id, r.org_id, r.affected_org_id, r.owner_user_id, r.status, r.revision
		FROM risk_items r
		WHERE r.status <> 'closed' AND (
			r.org_id IN (SELECT id FROM source_orgs) OR r.affected_org_id IN (SELECT id FROM source_orgs)
		)
	), local_memberships AS (
		SELECT m.org_id, m.user_id, m.is_primary, m.org_member_role, m.ticket_scope,
		       extract(epoch FROM m.expires_at) AS expires_at_epoch
		FROM user_orgs m JOIN scope_orgs o ON m.org_id=o.id
		WHERE m.source='local'
	), org_role_sources AS (
		SELECT gr.org_id, gr.role_id, r.parent_id, r.status, r.deleted_at IS NOT NULL AS deleted
		FROM org_roles gr JOIN scope_orgs o ON gr.org_id=o.id JOIN roles r ON r.id=gr.role_id
	)
	SELECT EXISTS (SELECT 1 FROM source_dept), EXISTS (SELECT 1 FROM approved_mapping),
		EXISTS (SELECT 1 FROM target_dept), jsonb_build_object(
		'approval_run_id', (SELECT run_id FROM approved_mapping),
		'source', (SELECT to_jsonb(s) FROM source_dept s),
		'target', (SELECT to_jsonb(t) FROM target_dept t),
		'virtual_groups', COALESCE((SELECT jsonb_agg(v ORDER BY v.id) FROM virtual_groups v), '[]'::jsonb),
		'in_flight_tickets', COALESCE((SELECT jsonb_agg(t ORDER BY t.id) FROM in_flight_tickets t), '[]'::jsonb),
		'in_flight_risks', COALESCE((SELECT jsonb_agg(r ORDER BY r.id) FROM in_flight_risks r), '[]'::jsonb),
		'local_memberships', COALESCE((SELECT jsonb_agg(m ORDER BY m.org_id,m.user_id) FROM local_memberships m), '[]'::jsonb),
		'org_roles', COALESCE((SELECT jsonb_agg(r ORDER BY r.org_id,r.role_id) FROM org_role_sources r), '[]'::jsonb)
	)`
}
