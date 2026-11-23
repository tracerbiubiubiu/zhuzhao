package ticket

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/exprcalc"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type evidenceTransitionRequest struct {
	ticketID          int64
	to, comment, key  string
	version, revision int64
	patch             json.RawMessage
	hash              [sha256.Size]byte
}

func (req evidenceTransitionRequest) versioned() bool { return req.revision > 0 }

// parseEvidenceTransitionRequest runs before acquiring C04 or ticket locks.
func parseEvidenceTransitionRequest(req *model.TransitionTicketRequest) (evidenceTransitionRequest, error) {
	cmd := evidenceTransitionRequest{ticketID: req.ID, to: req.To, comment: req.Comment, key: req.IdempotencyKey}
	if len(req.ExpectedRevision) == 0 && len(req.DefinitionVersion) == 0 && len(req.CustomDataPatch) == 0 && cmd.key == "" {
		return cmd, nil
	}
	var err error
	cmd.revision, err = parseEvidenceVersion(req.ExpectedRevision, "expected_revision")
	if err != nil {
		return evidenceTransitionRequest{}, err
	}
	cmd.version, err = parseEvidenceVersion(req.DefinitionVersion, "definition_version")
	if err != nil {
		return evidenceTransitionRequest{}, err
	}
	if cmd.key == "" || len(cmd.key) > 128 || strings.IndexFunc(cmd.key, func(r rune) bool { return r < '!' || r > '~' }) >= 0 {
		return evidenceTransitionRequest{}, errcode.New(errcode.ErrInvalidParams.Code, "Idempotency-Key 须为 1–128 个可打印 ASCII 字符，不含空格")
	}
	if len(cmd.comment) > RichtextMaxBytes {
		return evidenceTransitionRequest{}, errcode.New(errcode.ErrInvalidParams.Code, "comment 超出长度上限")
	}
	if len(req.CustomDataPatch) > CustomDataMaxBytes {
		return evidenceTransitionRequest{}, errcode.New(errcode.ErrInvalidParams.Code, "custom_data_patch 超出长度上限")
	}
	cmd.patch, err = normalizeTransitionPayload(req.CustomDataPatch, cmd.comment)
	if err != nil {
		return evidenceTransitionRequest{}, err
	}
	encoded, err := json.Marshal(struct {
		Contract string
		TicketID int64
		To       string
		Comment  string
		Revision int64
		Version  int64
		Patch    json.RawMessage
	}{"ticket.transition.evidence.v1", cmd.ticketID, cmd.to, cmd.comment, cmd.revision, cmd.version, cmd.patch})
	if err != nil {
		return evidenceTransitionRequest{}, errcode.ErrInternal
	}
	cmd.hash = sha256.Sum256(encoded)
	return cmd, nil
}

func parseEvidenceVersion(raw json.RawMessage, field string) (int64, error) {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		n, err := strconv.ParseInt(value, 10, 64)
		if err == nil && n > 0 && strconv.FormatInt(n, 10) == value {
			return n, nil
		}
	}
	return 0, errcode.New(errcode.ErrInvalidParams.Code, field+" 须为正的规范十进制字符串")
}

type evidenceCollectField struct {
	Key      string
	Required bool
}

// transitionCollectFields reads only the bound edge's collection contract.
func transitionCollectFields(raw json.RawMessage, from, to string) ([]evidenceCollectField, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var edges map[string]json.RawMessage
	if json.Unmarshal(raw, &edges) != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	edge, ok := edges[transitionKey(from, to)]
	if !ok {
		return nil, nil
	}
	return parseEvidenceCollectFields(edge)
}

func parseEvidenceCollectFields(edge json.RawMessage) ([]evidenceCollectField, error) {
	var meta struct {
		Fields json.RawMessage `json:"collect_fields"`
	}
	if json.Unmarshal(edge, &meta) != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if len(meta.Fields) == 0 {
		return nil, nil
	}
	var spec []struct {
		Key      string `json:"key"`
		Required *bool  `json:"required"`
	}
	if json.Unmarshal(meta.Fields, &spec) != nil || spec == nil {
		return nil, errcode.ErrServiceUnavailable
	}
	fields := make([]evidenceCollectField, 0, len(spec))
	seen := make(map[string]bool, len(spec))
	for _, item := range spec {
		if strings.TrimSpace(item.Key) == "" || item.Required == nil || seen[item.Key] {
			return nil, errcode.ErrServiceUnavailable
		}
		seen[item.Key] = true
		fields = append(fields, evidenceCollectField{Key: item.Key, Required: *item.Required})
	}
	return fields, nil
}

// rejectLegacyEvidenceTransition guards every entry that cannot submit evidence.
func rejectLegacyEvidenceTransition(raw json.RawMessage, from, to string, meta *TransitionMeta) error {
	fields, err := transitionCollectFields(raw, from, to)
	if err != nil {
		return err
	}
	if len(fields) > 0 || (meta != nil && meta.CommentRequired) {
		return errcode.New(errcode.ErrInvalidParams.Code, "此动作须经证据转换提交")
	}
	return nil
}

// findEvidenceTransitionReplayTx runs after current C04 and ticket authorization
// with the target ticket row locked, before checking the edge and original
// revision. A replay skips all writes and post-commit hooks. A nil result means
// the caller may start a new attempt.
func (s *Service) findEvidenceTransitionReplayTx(ctx context.Context, tx pgx.Tx, cmd evidenceTransitionRequest, actorID int64) (*repository.TicketEvidenceTransitionResult, error) {
	if !cmd.versioned() {
		return nil, nil
	}
	result, hash, err := s.ticketRepo.FindTicketEvidenceTransitionReceiptTx(ctx, tx, actorID, cmd.key)
	if err != nil || result == nil {
		return result, err
	}
	if !bytes.Equal(hash, cmd.hash[:]) {
		return nil, errcode.ErrConflict
	}
	return result, nil
}

// applyEvidenceTransitionTx consumes the current locked ticket and its pinned
// snapshot. The caller has acquired C04/type/ticket locks, checked L1/L2/L3 and
// the edge's execution condition on before, and ruled out a replay. The caller
// must roll back on any error and add any required successor intent before commit.
func (s *Service) applyEvidenceTransitionTx(ctx context.Context, tx pgx.Tx, cmd evidenceTransitionRequest, before *model.Ticket, snapshot *model.TicketDefinitionSnapshot, actorID int64) (repository.TicketEvidenceTransitionResult, error) {
	return s.applyEvidenceTransitionGuardedTx(ctx, tx, cmd, before, snapshot, actorID, nil)
}

func (s *Service) applyEvidenceTransitionGuardedTx(ctx context.Context, tx pgx.Tx, cmd evidenceTransitionRequest, before *model.Ticket, snapshot *model.TicketDefinitionSnapshot, actorID int64, guard func(*model.Ticket) error) (repository.TicketEvidenceTransitionResult, error) {
	var result repository.TicketEvidenceTransitionResult
	if !cmd.versioned() || before.ID != cmd.ticketID {
		return result, errcode.ErrInvalidParams
	}
	if snapshot == nil || before.DefinitionVersion == nil || snapshot.Type.Code != before.TypeCode {
		return result, errcode.ErrServiceUnavailable
	}
	if *before.DefinitionVersion != cmd.version {
		return result, errcode.ErrConcurrentModification
	}
	revision, err := s.ticketRepo.ReadTicketEvidenceRevisionTx(ctx, tx, before.ID)
	if err != nil {
		return result, err
	}
	if revision != cmd.revision {
		return result, errcode.ErrConcurrentModification
	}
	sm, err := FromTicketType(&snapshot.Type)
	if err != nil {
		return result, errcode.ErrServiceUnavailable
	}
	if err := sm.AssertTransition(before.Status, cmd.to); err != nil {
		return result, err
	}
	tm, err := parseTransitionMeta(snapshot.Type.TransitionMeta, before.Status, cmd.to)
	if err != nil {
		return result, errcode.ErrServiceUnavailable
	}
	if tm != nil && tm.CommentRequired && strings.TrimSpace(cmd.comment) == "" {
		return result, errcode.New(errcode.ErrInvalidParams.Code, "请填写办理说明")
	}
	collect, err := transitionCollectFields(snapshot.Type.TransitionMeta, before.Status, cmd.to)
	if err != nil {
		return result, err
	}
	data, err := s.validateEvidenceTransitionData(ctx, tx, before, snapshot.Fields, collect, cmd.patch, tm)
	if err != nil {
		return result, err
	}
	if guard != nil {
		candidate := cloneTicketSnapshot(before)
		candidate.CustomData = append(json.RawMessage(nil), data...)
		if err := guard(&candidate); err != nil {
			return result, err
		}
	}
	if len(collect) > 0 {
		refs, err := fieldReferences(snapshot.Fields, data)
		if err != nil {
			return result, err
		}
		if err := s.bindUpdateReferencesTx(ctx, tx, before.ID, actorID, nil, refs); err != nil {
			return result, err
		}
	}
	written, err := s.ticketRepo.ApplyTicketEvidenceTransitionTx(ctx, tx, before.ID, cmd.revision, cmd.version, before.Status, cmd.to, data)
	if err != nil {
		return result, err
	}
	event := model.TicketEvent{TicketID: before.ID, UserID: actorID, Action: EventUpdated}
	if before.Status != cmd.to {
		after := *before
		if err := s.applyStateSLARuleTx(ctx, tx, &snapshot.Type, &after, cmd.to); err != nil {
			return result, err
		}
		event.Action, event.FromValue, event.ToValue = EventStatusChanged, before.Status, cmd.to
	}
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &event); err != nil {
		return result, err
	}
	if err := s.writeLifecycleCommentTx(ctx, tx, before.ID, actorID, cmd.comment); err != nil {
		return result, err
	}
	if err := s.ticketRepo.RecordTicketEvidenceTransitionReceiptTx(ctx, tx, before.ID, actorID, cmd.key, cmd.hash[:], written); err != nil {
		return result, err
	}
	return written, nil
}

func (s *Service) validateEvidenceTransitionData(ctx context.Context, q repository.RowQuerier, before *model.Ticket, fields []*model.TicketTypeField, collect []evidenceCollectField, patch json.RawMessage, tm *TransitionMeta) (json.RawMessage, error) {
	var submitted map[string]json.RawMessage
	if json.Unmarshal(patch, &submitted) != nil || submitted == nil {
		return nil, errcode.ErrInvalidParams
	}
	if len(collect) == 0 {
		if len(submitted) > 0 {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, "此动作不接收 custom_data_patch")
		}
		return before.CustomData, checkValidator(withDictQuerier(ctx, q), tm, before)
	}
	// G2 expands dictionary options in place. Keep the pinned schema immutable.
	schema := make([]*model.TicketTypeField, 0, len(fields))
	byKey := make(map[string]*model.TicketTypeField, len(fields))
	protected := make(map[string]bool)
	for _, field := range fields {
		if field == nil || field.FieldKey == "" || byKey[field.FieldKey] != nil {
			return nil, errcode.ErrServiceUnavailable
		}
		var authority struct {
			WriteAuthority string `json:"write_authority"`
		}
		if len(field.FieldMeta) > 0 && json.Unmarshal(field.FieldMeta, &authority) != nil {
			return nil, errcode.ErrServiceUnavailable
		}
		if authority.WriteAuthority != "" {
			if formulaOf(field.FieldMeta) != "" {
				return nil, errcode.ErrServiceUnavailable
			}
			protected[field.FieldKey] = true
		}
		copy := *field
		schema = append(schema, &copy)
		byKey[field.FieldKey] = &copy
	}
	for _, field := range schema {
		if visibility := visibleWhenOf(field.FieldMeta); visibility != "" {
			ids, err := exprcalc.Identifiers(visibility)
			if err != nil {
				return nil, errcode.ErrServiceUnavailable
			}
			for _, id := range ids {
				if dependency := byKey[id]; dependency == nil || formulaOf(dependency.FieldMeta) != "" {
					return nil, errcode.ErrServiceUnavailable
				}
			}
		}
	}
	allowed := make(map[string]bool, len(collect))
	for _, item := range collect {
		field := byKey[item.Key]
		if field == nil || !validFieldTypes[field.FieldType] || field.FieldType == "tips" || protected[item.Key] || formulaOf(field.FieldMeta) != "" {
			return nil, errcode.ErrServiceUnavailable
		}
		allowed[item.Key] = true
	}
	for key := range submitted {
		if !allowed[key] {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, "custom_data_patch 含此动作不可编辑的字段")
		}
	}
	merged, err := mergeCustomData(before.CustomData, patch)
	if err != nil {
		return nil, errcode.ErrInvalidParams
	}
	var candidate map[string]json.RawMessage
	if json.Unmarshal(merged, &candidate) != nil {
		return nil, errcode.ErrInvalidParams
	}
	// Preserve unknown legacy keys and protected values across G2's pruning.
	// Unknown keys and stale formulas cannot enter the expression environment.
	legacyValues := make(map[string]json.RawMessage)
	protectedValues := make(map[string]json.RawMessage)
	for key, value := range candidate {
		field := byKey[key]
		switch {
		case field == nil:
			legacyValues[key] = value
			delete(candidate, key)
		case protected[key]:
			protectedValues[key] = value
		case formulaOf(field.FieldMeta) != "":
			delete(candidate, key)
		}
	}
	input, err := json.Marshal(candidate)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	if err := checkEvidenceVisibility(ctx, q, byKey, collect, submitted, input); err != nil {
		return nil, err
	}
	if err := s.resolveEvidenceOptions(ctx, q, schema); err != nil {
		return nil, err
	}
	normalized, err := s.validateCustomData(ctx, q, before.TypeCode, input, before.CustomData, schema)
	if err != nil {
		return nil, err
	}
	var valid map[string]json.RawMessage
	if json.Unmarshal(normalized, &valid) != nil {
		return nil, errcode.ErrInternal
	}
	for _, item := range collect {
		if item.Required && evidenceValueEmpty(valid[item.Key]) {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, "缺少必填证据")
		}
	}
	for key, value := range submitted {
		if valid[key] == nil && string(value) != "null" {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, "此动作含当前不可见的证据字段")
		}
	}
	for key := range protected {
		delete(valid, key)
	}
	for key, value := range protectedValues {
		valid[key] = value
	}
	candidateTicket := *before
	candidateTicket.CustomData, err = json.Marshal(valid)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	if err := checkValidator(withDictQuerier(ctx, q), tm, &candidateTicket); err != nil {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "当前证据未满足动作前置条件")
	}
	for key, value := range legacyValues {
		valid[key] = value
	}
	data, err := json.Marshal(valid)
	if err != nil {
		return nil, errcode.ErrInternal
	}
	if len(data) > CustomDataMaxBytes {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "合并后的 custom_data 超出长度上限")
	}
	return data, nil
}

// Materialize live dictionary values before G2, whose compatibility wrapper
// skips membership checks on dictionary read failures.
func (s *Service) resolveEvidenceOptions(ctx context.Context, q repository.RowQuerier, schema []*model.TicketTypeField) error {
	for _, field := range schema {
		switch field.FieldType {
		case "select", "radio", "multi_select":
		default:
			continue
		}
		raw := field.FieldOptions
		if _, dict := dictOptionReference(raw); dict {
			if q == nil {
				return errcode.ErrServiceUnavailable
			}
			expanded, err := s.expandDictOptionsStrict(ctx, q, raw)
			if err != nil {
				return errcode.ErrServiceUnavailable
			}
			// Object options distinguish actual codes from a dict: selector.
			var codes []string
			if json.Unmarshal(expanded, &codes) != nil {
				return errcode.ErrServiceUnavailable
			}
			type option struct {
				Value string `json:"value"`
			}
			options := make([]option, 0, len(codes))
			for _, code := range codes {
				options = append(options, option{Value: code})
			}
			raw, err = json.Marshal(options)
			if err != nil {
				return errcode.ErrInternal
			}
		}
		if values, err := optionValues(raw); err != nil || values == nil {
			return errcode.ErrServiceUnavailable
		}
		field.FieldOptions = raw
	}
	return nil
}

func checkEvidenceVisibility(ctx context.Context, q repository.RowQuerier, byKey map[string]*model.TicketTypeField, collect []evidenceCollectField, submitted map[string]json.RawMessage, input json.RawMessage) error {
	var fields map[string]any
	if json.Unmarshal(input, &fields) != nil {
		return errcode.ErrInvalidParams
	}
	env := exprcalc.EvalEnv{Fields: fields, Context: withDictQuerier(ctx, q)}
	for _, item := range collect {
		raw, supplied := submitted[item.Key]
		if item.Required && evidenceValueEmpty(raw) {
			return errcode.New(errcode.ErrInvalidParams.Code, "缺少必填证据")
		}
		if !supplied {
			continue
		}
		if expr := visibleWhenOf(byKey[item.Key].FieldMeta); expr != "" {
			value, err := exprcalc.EvalFormula(expr, env)
			visible, boolean := value.(bool)
			if err != nil || !boolean {
				return errcode.ErrServiceUnavailable
			}
			if !visible {
				return errcode.New(errcode.ErrInvalidParams.Code, "此动作含当前不可见的证据字段")
			}
		}
	}
	return nil
}

func evidenceValueEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte("[]")) {
		return true
	}
	var text string
	return json.Unmarshal(raw, &text) == nil && strings.TrimSpace(text) == ""
}

// cloneTicketSnapshot keeps trusted callbacks and queued effects from changing
// the command's original authorization/validator snapshot through pointer fields.
func cloneTicketSnapshot(ticket *model.Ticket) model.Ticket {
	out := *ticket
	out.CustomData = append(json.RawMessage(nil), ticket.CustomData...)
	out.DefinitionVersion = cloneTicketPointer(ticket.DefinitionVersion)
	out.DefinitionRef = cloneTicketPointer(ticket.DefinitionRef)
	out.AssignedTo = cloneTicketPointer(ticket.AssignedTo)
	out.EscalationSupervisor = cloneTicketPointer(ticket.EscalationSupervisor)
	out.SLADueAt = cloneTicketPointer(ticket.SLADueAt)
	out.StatusChangedAt = cloneTicketPointer(ticket.StatusChangedAt)
	return out
}

func cloneTicketPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
