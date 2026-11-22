package admincli

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type bindTicketDefinitionManifest struct {
	TicketID                    int64     `json:"ticket_id,string"`
	TypeCode                    string    `json:"type_code"`
	OriginalCreatedBy           int64     `json:"original_created_by,string"`
	OriginalCreatedAt           time.Time `json:"original_created_at"`
	ExpectedStatus              string    `json:"expected_status"`
	DefinitionVersion           int64     `json:"definition_version,string"`
	DefinitionHash              string    `json:"definition_hash"`
	ConfigurationEvidenceSHA256 string    `json:"configuration_evidence_sha256"`
	MappingEvidenceSHA256       string    `json:"mapping_evidence_sha256"`
	ApprovalSHA256              string    `json:"approval_sha256"`
	TicketEvidenceSHA256        string    `json:"ticket_evidence_sha256"`
}

type bindTicketDefinitionReceipt struct {
	bindTicketDefinitionManifest
	BoundAt time.Time `json:"bound_at"`
}

const bindTicketManifestLimit = 8192

var lowercaseSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func readBindTicketDefinitionManifest(input io.Reader) (bindTicketDefinitionManifest, error) {
	var manifest bindTicketDefinitionManifest
	reader := &io.LimitedReader{R: input, N: bindTicketManifestLimit + 1}
	if err := jsonv2.UnmarshalRead(reader, &manifest, jsonv2.RejectUnknownMembers(true)); err != nil || reader.N == 0 {
		return manifest, errors.New("清单须为单个完整的 JSON 对象，字段名唯一且符合命令帮助")
	}
	if manifest.TicketID <= 0 || manifest.DefinitionVersion <= 0 || manifest.OriginalCreatedBy <= 0 ||
		manifest.OriginalCreatedAt.IsZero() || manifest.OriginalCreatedAt.Nanosecond()%1000 != 0 ||
		manifest.TypeCode == "" || manifest.TypeCode != strings.TrimSpace(manifest.TypeCode) || utf8.RuneCountInString(manifest.TypeCode) > 50 ||
		manifest.ExpectedStatus == "" || manifest.ExpectedStatus != strings.TrimSpace(manifest.ExpectedStatus) || utf8.RuneCountInString(manifest.ExpectedStatus) > 20 {
		return manifest, errors.New("清单缺少有效的票标识、创建事实、预期状态或定义版本")
	}
	for _, digest := range []string{manifest.DefinitionHash, manifest.ConfigurationEvidenceSHA256,
		manifest.MappingEvidenceSHA256, manifest.ApprovalSHA256, manifest.TicketEvidenceSHA256} {
		if !lowercaseSHA256.MatchString(digest) {
			return manifest, errors.New("清单须提供完整定义、旧配置、映射、批准及原票的 SHA-256 摘要")
		}
	}
	manifest.OriginalCreatedAt = manifest.OriginalCreatedAt.UTC()
	return manifest, nil
}

// RunBindTicketDefinitions applies one independently approved legacy mapping.
// The operator authenticates the evidence offline. The command verifies the
// stored definition against the approved complete snapshot hash and compares
// the original ticket facts before recording the binding and evidence digests.
func RunBindTicketDefinitions(ctx context.Context, databaseURL string, input io.Reader, stdout io.Writer) error {
	manifest, err := readBindTicketDefinitionManifest(input)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return bindTicketDBError("连接数据库", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return bindTicketDBError("开启补绑事务", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	receipt, err := bindTicketDefinition(ctx, tx, manifest)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return bindTicketDBError("提交补绑结果，使用原清单重试以核对结果", err)
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
		return errors.New("数据库内的补绑结果已确认，回执输出失败，请用原清单重试")
	}
	return nil
}

func bindTicketDefinition(ctx context.Context, tx pgx.Tx, manifest bindTicketDefinitionManifest) (bindTicketDefinitionReceipt, error) {
	var receipt bindTicketDefinitionReceipt
	var typeCode string
	var createdBy int64
	var status *string
	var createdAt *time.Time
	var boundVersion *int64
	var businessHash string
	// Lock the same row as business writes. The definition is immutable and
	// addressed directly, so the live type row and its current pointer stay out
	// of this transaction's row lock order.
	ticketErr := tx.QueryRow(ctx, `SELECT type_code, created_by, created_at, status, definition_version,
		ticket_definition_snapshot_hash(to_jsonb(t)-'definition_version')
		FROM tickets t WHERE id=$1 FOR UPDATE`, manifest.TicketID).Scan(&typeCode, &createdBy, &createdAt, &status, &boundVersion, &businessHash)
	if ticketErr != nil && !errors.Is(ticketErr, pgx.ErrNoRows) {
		return receipt, bindTicketDBError("锁定工单", ticketErr)
	}

	// Read at READ COMMITTED after taking the ticket lock so simultaneous
	// retries see the winning receipt. A receipt also survives ticket deletion.
	receipt.TicketID = manifest.TicketID
	err := tx.QueryRow(ctx, `SELECT type_code, original_created_by, original_created_at, expected_status,
		definition_version, definition_hash, configuration_evidence_sha256, mapping_evidence_sha256,
		approval_sha256, ticket_evidence_sha256, bound_at
		FROM ticket_legacy_definition_bindings WHERE ticket_id=$1`, manifest.TicketID).Scan(
		&receipt.TypeCode, &receipt.OriginalCreatedBy, &receipt.OriginalCreatedAt, &receipt.ExpectedStatus,
		&receipt.DefinitionVersion, &receipt.DefinitionHash, &receipt.ConfigurationEvidenceSHA256,
		&receipt.MappingEvidenceSHA256, &receipt.ApprovalSHA256, &receipt.TicketEvidenceSHA256, &receipt.BoundAt)
	if err == nil {
		receipt.OriginalCreatedAt = receipt.OriginalCreatedAt.UTC()
		receipt.BoundAt = receipt.BoundAt.UTC()
		if receipt.bindTicketDefinitionManifest != manifest {
			return receipt, errors.New("清单与历史补绑回执冲突")
		}
		return receipt, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return receipt, bindTicketDBError("读取补绑回执", err)
	}
	if ticketErr != nil {
		return receipt, errors.New("未找到指定工单")
	}
	if boundVersion != nil {
		return receipt, errors.New("工单已有定义引用，且无匹配的旧票补绑回执")
	}
	if typeCode != manifest.TypeCode || createdBy != manifest.OriginalCreatedBy || createdAt == nil || !createdAt.Equal(manifest.OriginalCreatedAt) {
		return receipt, errors.New("原票事实与清单不匹配")
	}
	if status == nil || *status != manifest.ExpectedStatus {
		return receipt, errors.New("当前工单状态与清单不匹配")
	}
	if *status == ticket.StatusClosed || *status == ticket.StatusRejected {
		return receipt, errors.New("终态工单不能补绑")
	}
	if err := verifyLegacyTicketDefinition(ctx, tx, manifest); err != nil {
		return receipt, err
	}
	tag, err := tx.Exec(ctx, `UPDATE tickets SET definition_version=$2 WHERE id=$1 AND definition_version IS NULL`,
		manifest.TicketID, manifest.DefinitionVersion)
	if err != nil {
		return receipt, bindTicketDBError("补绑工单定义", err)
	}
	if tag.RowsAffected() != 1 {
		return receipt, errors.New("工单定义未能补绑")
	}
	// Check the row after triggers complete, leaving business fields in PostgreSQL.
	var updatedHash string
	var updatedVersion *int64
	err = tx.QueryRow(ctx, `SELECT definition_version, ticket_definition_snapshot_hash(to_jsonb(t)-'definition_version')
		FROM tickets t WHERE id=$1`, manifest.TicketID).Scan(&updatedVersion, &updatedHash)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (updatedVersion == nil || *updatedVersion != manifest.DefinitionVersion) {
		return receipt, errors.New("工单定义未能补绑")
	}
	if err != nil {
		return receipt, bindTicketDBError("复核补绑后的工单", err)
	}
	if updatedHash != businessHash {
		return receipt, errors.New("补绑触发了工单业务字段变化，已回滚")
	}

	receipt.bindTicketDefinitionManifest = manifest
	err = tx.QueryRow(ctx, `INSERT INTO ticket_legacy_definition_bindings
		(ticket_id, type_code, original_created_by, original_created_at, expected_status, definition_version,
		 definition_hash, configuration_evidence_sha256, mapping_evidence_sha256, approval_sha256, ticket_evidence_sha256)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING bound_at`,
		manifest.TicketID, manifest.TypeCode, manifest.OriginalCreatedBy, manifest.OriginalCreatedAt,
		manifest.ExpectedStatus, manifest.DefinitionVersion, manifest.DefinitionHash, manifest.ConfigurationEvidenceSHA256,
		manifest.MappingEvidenceSHA256, manifest.ApprovalSHA256, manifest.TicketEvidenceSHA256).Scan(&receipt.BoundAt)
	if err != nil {
		return receipt, bindTicketDBError("保存补绑回执", err)
	}
	receipt.BoundAt = receipt.BoundAt.UTC()
	return receipt, nil
}

func verifyLegacyTicketDefinition(ctx context.Context, tx pgx.Tx, manifest bindTicketDefinitionManifest) error {
	var hash, computedHash string
	var format int
	var raw json.RawMessage
	err := tx.QueryRow(ctx, `SELECT definition_hash, ticket_definition_snapshot_hash(snapshot), schema_format_version, snapshot
		FROM ticket_type_definitions WHERE type_code=$1 AND definition_version=$2`, manifest.TypeCode, manifest.DefinitionVersion).
		Scan(&hash, &computedHash, &format, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("同类型的指定旧定义不存在")
	}
	if err != nil {
		return bindTicketDBError("核对旧定义", err)
	}
	if format != 1 || hash != manifest.DefinitionHash || computedHash != manifest.DefinitionHash {
		return errors.New("指定定义的格式或完整快照摘要不匹配")
	}
	return checkLegacyDefinitionSnapshot(raw, manifest.TypeCode, manifest.ExpectedStatus)
}

func checkLegacyDefinitionSnapshot(raw json.RawMessage, typeCode, status string) error {
	snapshot, err := repository.DecodeBoundTicketDefinition(typeCode, raw)
	if err != nil {
		return errors.New("旧定义快照不完整或类型不匹配")
	}

	var states []string
	var transitions map[string][]string
	if json.Unmarshal(snapshot.Type.States, &states) != nil || json.Unmarshal(snapshot.Type.Transitions, &transitions) != nil {
		return errors.New("旧定义快照不完整或类型不匹配")
	}
	if !slices.Contains(states, status) {
		return errors.New("原票状态不在已验证的旧定义中")
	}
	for _, next := range transitions[status] {
		if next != status {
			return nil
		}
	}
	return errors.New("终态工单不能补绑")
}

// PostgreSQL details can contain configuration values, ticket data or the DSN.
// Preserve the safe SQLSTATE for local diagnosis without returning those details.
func bindTicketDBError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Errorf("%s失败（SQLSTATE %s）", operation, pgErr.Code)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s失败（%w）", operation, err)
	}
	return errors.New(operation + "失败")
}
