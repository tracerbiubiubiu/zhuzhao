package ticket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	definitionPlanPath    = "/api/v1/ticket-types/definitions/plan"
	definitionPublishPath = "/api/v1/ticket-types/definitions/publish"
	definitionTypePath    = "/api/v1/ticket-types"
)

// DefinitionPublicationError supplies a stable response reason and the
// existing business error code for conflicts and unavailable definitions.
type DefinitionPublicationError struct {
	Reason string
	Cause  *errcode.Error
}

func (e *DefinitionPublicationError) Error() string { return e.Cause.Error() }
func (e *DefinitionPublicationError) Unwrap() error { return e.Cause }

func definitionConflict(reason string) error {
	return &DefinitionPublicationError{Reason: reason, Cause: errcode.ErrConcurrentModification}
}

type definitionPublicationTarget struct {
	Code             string
	DraftRevision    int64
	PublishedVersion *int64
}

func parseDefinitionPublicationTarget(req model.TicketDefinitionPlanRequest) (definitionPublicationTarget, error) {
	revision, err := definitionPositiveDecimal(req.ExpectedDraftRevision)
	if err != nil || req.Code == "" || len([]rune(req.Code)) > 50 {
		return definitionPublicationTarget{}, errcode.ErrInvalidParams
	}
	target := definitionPublicationTarget{Code: req.Code, DraftRevision: revision}
	if req.ExpectedPublishedVersion != nil && *req.ExpectedPublishedVersion != "" {
		v, err := definitionPositiveDecimal(*req.ExpectedPublishedVersion)
		if err != nil {
			return definitionPublicationTarget{}, err
		}
		target.PublishedVersion = &v
	}
	return target, nil
}

func definitionPositiveDecimal(raw string) (int64, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != raw {
		return 0, errcode.ErrInvalidParams
	}
	return n, nil
}

func definitionFingerprint(input any) (string, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func validDefinitionOperationID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if c < '!' || c > '~' || c == ',' {
			return false
		}
	}
	return true
}

// withDefinitionHTTP binds the service method to the authenticated route,
// then rechecks the credential and current L1 in the IAM transaction.
func (s *Service) withDefinitionHTTP(ctx context.Context, actor int64, method, path string, run func(context.Context, pgx.Tx, middleware.IAMRequestPurpose) error) error {
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || actor <= 0 || purpose.UserID != actor || purpose.Method != method || purpose.Path != path || purpose.SelfService {
		return errcode.ErrNoPermission
	}
	return repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.freshCommandCredential(ctx, actor, purpose); err != nil {
			return err
		}
		if _, err := s.freshCommandRoles(ctx, actor, purpose.Model, ticketPolicyRoute{path: path, method: method}); err != nil {
			return err
		}
		return run(ctx, tx, purpose)
	})
}

func (s *Service) withDefinitionPublisher(ctx context.Context, actor int64, path string, run func(context.Context, pgx.Tx) error) error {
	return s.withDefinitionHTTP(ctx, actor, "POST", path, func(ctx context.Context, tx pgx.Tx, _ middleware.IAMRequestPurpose) error {
		allowed, err := s.ticketRepo.HasDefinitionConfigMenu(ctx, actor, true)
		if err != nil {
			return err
		}
		if !allowed {
			return errcode.ErrNoPermission
		}
		return run(ctx, tx)
	})
}

func (s *Service) PlanTicketDefinition(ctx context.Context, req model.TicketDefinitionPlanRequest, actor int64) (*model.TicketDefinitionPlan, error) {
	target, err := parseDefinitionPublicationTarget(req)
	if err != nil {
		return nil, err
	}
	var result *model.TicketDefinitionPlan
	err = s.withDefinitionPublisher(ctx, actor, definitionPlanPath, func(ctx context.Context, tx pgx.Tx) error {
		draft, fields, err := s.lockPlannedDraft(ctx, tx, target, 0)
		if err != nil {
			return err
		}
		hash, err := s.previewDefinitionSnapshot(ctx, tx, draft, fields)
		if err != nil {
			return err
		}
		refs, err := s.definitionReferences(ctx, tx, draft, fields)
		if err != nil {
			return err
		}
		plan := repository.DefinitionPublicationPlan{
			ActorID: actor, TypeID: draft.ID, TypeCode: target.Code, ExpectedPublishedVersion: target.PublishedVersion,
			SnapshotHash: hash,
			TicketDefinitionPlan: model.TicketDefinitionPlan{
				DraftRevision: draft.Version, Changed: true, ReferenceRevisions: refs, Warnings: []model.TicketDefinitionWarning{},
			},
		}
		if draft.CurrentPublishedVersion != nil {
			current, err := s.ticketRepo.GetTicketDefinitionTx(ctx, tx, draft.Code, *draft.CurrentPublishedVersion)
			if err != nil {
				return err
			}
			prior, err := decodePublishedDefinition(current)
			if err != nil {
				return err
			}
			ref := publicationDefinitionRef(current)
			plan.CurrentDefinition = &ref
			plan.Changed = hash != current.DefinitionHash
			plan.Warnings = definitionRequiredWarnings(prior.Fields, fields)
		}
		plan.CanonicalHash, err = definitionFingerprint(struct {
			Contract      string
			Actor, TypeID int64
			Target        definitionPublicationTarget
			SnapshotHash  string
			References    map[string]model.TicketDefinitionReferenceRevision
			Warnings      []model.TicketDefinitionWarning
		}{"ticket.definition.publish.v1", actor, draft.ID, target, hash, refs, plan.Warnings})
		if err != nil {
			return err
		}
		if err := s.ticketRepo.InsertDefinitionPublicationPlan(ctx, tx, &plan); err != nil {
			return err
		}
		result = &plan.TicketDefinitionPlan
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) ExecuteTicketDefinitionPublication(ctx context.Context, req model.TicketDefinitionPublishRequest, actor int64, operationID string) (*model.TicketDefinitionPublicationReceipt, error) {
	target, err := parseDefinitionPublicationTarget(req.TicketDefinitionPlanRequest)
	if err != nil {
		return nil, err
	}
	planID, err := definitionPositiveDecimal(req.PlanID)
	hash, hashErr := hex.DecodeString(req.CanonicalHash)
	if err != nil || hashErr != nil || len(hash) != sha256.Size || hex.EncodeToString(hash) != req.CanonicalHash || !validDefinitionOperationID(operationID) {
		return nil, errcode.ErrInvalidParams
	}
	ack := append([]string{}, req.AcknowledgedWarningIDs...)
	slices.Sort(ack)
	requestHash, err := definitionFingerprint(struct {
		Contract             string
		Target               definitionPublicationTarget
		PlanID               int64
		PlanHash             string
		AcknowledgedWarnings []string
	}{"ticket.definition.publish.v1", target, planID, req.CanonicalHash, ack})
	if err != nil {
		return nil, err
	}

	var result *model.TicketDefinitionPublicationReceipt
	err = s.withDefinitionPublisher(ctx, actor, definitionPublishPath, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.ticketRepo.LockDefinitionPublicationOperation(ctx, tx, actor, operationID); err != nil {
			return err
		}
		receipt, originalHash, err := s.ticketRepo.GetDefinitionPublicationReceipt(ctx, tx, actor, operationID)
		if err != nil {
			return err
		}
		if receipt != nil {
			if originalHash != requestHash {
				return definitionConflict("operation_payload_changed")
			}
			result = receipt
			return nil
		}

		plan, err := s.ticketRepo.GetDefinitionPublicationPlan(ctx, tx, actor, planID)
		if errors.Is(err, errcode.ErrConcurrentModification) {
			return definitionConflict("plan_mismatch")
		}
		if err != nil {
			return err
		}
		if plan.TypeCode != target.Code || plan.DraftRevision != target.DraftRevision ||
			!publicationVersionEquals(plan.ExpectedPublishedVersion, target.PublishedVersion) || plan.CanonicalHash != req.CanonicalHash {
			return definitionConflict("plan_mismatch")
		}
		warningIDs := make([]string, 0, len(plan.Warnings))
		for _, warning := range plan.Warnings {
			warningIDs = append(warningIDs, warning.ID)
		}
		slices.Sort(warningIDs)
		if !slices.Equal(warningIDs, ack) {
			return definitionConflict("warning_acknowledgement_required")
		}
		draft, fields, err := s.lockPlannedDraft(ctx, tx, target, plan.TypeID)
		if err != nil {
			return err
		}
		refs, err := s.definitionReferences(ctx, tx, draft, fields)
		if err != nil {
			return err
		}
		if !maps.Equal(refs, plan.ReferenceRevisions) {
			return definitionConflict("reference_changed")
		}
		// PublishTicketType joins the owning IAM transaction. The plan, rule,
		// pointer and receipt checks all run before that transaction commits.
		def, changed, err := s.PublishTicketType(ctx, draft, actor)
		if err != nil {
			return err
		}
		if def.DefinitionHash != plan.SnapshotHash {
			return definitionConflict("draft_changed")
		}
		result = &model.TicketDefinitionPublicationReceipt{
			OperationID: operationID, Changed: changed, DefinitionRef: publicationDefinitionRef(def), DraftRevision: draft.Version,
		}
		return s.ticketRepo.InsertDefinitionPublicationReceipt(ctx, tx, actor, planID, requestHash, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) lockPlannedDraft(ctx context.Context, tx pgx.Tx, target definitionPublicationTarget, typeID int64) (*model.TicketType, []*model.TicketTypeField, error) {
	current, err := s.ticketRepo.GetTicketType(ctx, target.Code)
	if errors.Is(err, errcode.ErrTicketTypeNotFound) && typeID != 0 {
		return nil, nil, definitionConflict("draft_changed")
	}
	if err != nil {
		return nil, nil, err
	}
	if (typeID != 0 && (current.ID != typeID || !current.IsActive)) || current.Version != target.DraftRevision {
		return nil, nil, definitionConflict("draft_changed")
	}
	if !publicationVersionEquals(current.CurrentPublishedVersion, target.PublishedVersion) {
		return nil, nil, definitionConflict("published_pointer_changed")
	}
	draft, fields, err := s.ticketRepo.LockDefinitionDraftTx(ctx, tx, current)
	if errors.Is(err, errcode.ErrConcurrentModification) || typeID != 0 && errors.Is(err, errcode.ErrTicketTypeNotFound) {
		return nil, nil, definitionConflict("draft_or_pointer_changed")
	}
	return draft, fields, err
}

func publicationVersionEquals(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func publicationDefinitionRef(def *model.TicketTypeDefinition) model.TicketDefinitionRef {
	return model.TicketDefinitionRef{TypeCode: def.TypeCode, DefinitionVersion: def.DefinitionVersion, DefinitionHash: def.DefinitionHash}
}

func decodePublishedDefinition(def *model.TicketTypeDefinition) (*model.TicketDefinitionSnapshot, error) {
	if def.SchemaFormatVersion != 1 {
		return nil, errcode.ErrServiceUnavailable
	}
	return repository.DecodeBoundTicketDefinition(def.TypeCode, def.Snapshot)
}

func definitionRequiredWarnings(prior, fields []*model.TicketTypeField) []model.TicketDefinitionWarning {
	required := make(map[string]bool, len(prior))
	for _, f := range prior {
		if f != nil {
			required[f.FieldKey] = f.Required
		}
	}
	warnings := make([]model.TicketDefinitionWarning, 0)
	for _, f := range fields {
		if f.Required && !required[f.FieldKey] {
			warnings = append(warnings, model.TicketDefinitionWarning{
				ID: "required:" + f.FieldKey, FieldKey: f.FieldKey, Message: "新增必填要求可能影响现有建单客户端",
			})
		}
	}
	return warnings
}

func (s *Service) definitionReferences(ctx context.Context, tx pgx.Tx, draft *model.TicketType, fields []*model.TicketTypeField) (map[string]model.TicketDefinitionReferenceRevision, error) {
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(draft.TransitionMeta, &meta); err != nil {
		return nil, errcode.ErrInvalidParams
	}
	templates := make(map[string]struct{})
	dictionaries := make(definitionDictionaryReferences)
	for _, field := range fields {
		if err := dictionaries.collect(formulaOf(field.FieldMeta), visibleWhenOf(field.FieldMeta)); err != nil {
			return nil, err
		}
	}
	for edge, raw := range meta {
		if !strings.Contains(edge, ">") {
			continue
		}
		var tm TransitionMeta
		if err := json.Unmarshal(raw, &tm); err != nil {
			return nil, errcode.ErrInvalidParams
		}
		if err := dictionaries.collect(tm.Validator); err != nil {
			return nil, err
		}
		for _, action := range parsePostActions(&tm) {
			if code, ok := action.Params["template"].(string); action.Name == "notify" && ok && code != "" {
				templates[code] = struct{}{}
			}
		}
	}
	refs, err := s.ticketRepo.LockDefinitionReferenceRevisions(ctx, tx, fields, dictionaries, slices.Sorted(maps.Keys(templates)))
	if errors.Is(err, errcode.ErrConcurrentModification) {
		return nil, definitionConflict("reference_changed")
	}
	return refs, err
}

// Dynamic lookup keys remain runtime inputs. Only a literal first argument to
// lookup_dict designates a dictionary that can be checked at publication time.
type definitionDictionaryReferences map[string]struct{}

func (refs definitionDictionaryReferences) collect(expressions ...string) error {
	for _, expr := range expressions {
		if expr == "" {
			continue
		}
		tree, err := parser.Parse(expr)
		if err != nil {
			return errcode.ErrInvalidParams
		}
		ast.Walk(&tree.Node, refs)
	}
	return nil
}

func (refs definitionDictionaryReferences) Visit(node *ast.Node) {
	call, ok := (*node).(*ast.CallNode)
	if !ok || len(call.Arguments) != 2 {
		return
	}
	callee, ok := call.Callee.(*ast.IdentifierNode)
	if !ok || callee.Value != "lookup_dict" {
		return
	}
	if literal, ok := call.Arguments[0].(*ast.StringNode); ok {
		refs[literal.Value] = struct{}{}
	}
}

// The plan uses the same stored graph, field validators and canonical database
// hash as publication, without writing a candidate definition or a pointer.
func (s *Service) previewDefinitionSnapshot(ctx context.Context, tx pgx.Tx, draft *model.TicketType, fields []*model.TicketTypeField) (string, error) {
	if err := validateTypeGraph(draft.States, draft.Transitions); err != nil {
		return "", err
	}
	var states []string
	if err := json.Unmarshal(draft.States, &states); err != nil {
		return "", errcode.ErrInvalidParams
	}
	if err := validateTransitionMeta(states, draft.TransitionMeta); err != nil {
		return "", err
	}
	var edges map[string][]string
	if err := json.Unmarshal(draft.Transitions, &edges); err != nil {
		return "", errcode.ErrInvalidParams
	}
	for _, targets := range edges {
		seen := make(map[string]bool, len(targets))
		for _, to := range targets {
			if seen[to] {
				return "", errcode.New(errcode.ErrInvalidParams.Code, "同一来源状态不能有重复的目标状态")
			}
			seen[to] = true
		}
	}
	if err := validateTransitionEdges(states, edges, draft.TransitionMeta, s.actionReg.validatePublishedPostActions); err != nil {
		return "", err
	}
	inputs := make([]model.TicketTypeFieldInput, 0, len(fields))
	snapshotFields := make([]*model.TicketTypeField, len(fields))
	if fields == nil {
		snapshotFields = nil
	}
	for i, f := range fields {
		if f.FieldID <= 0 {
			return "", errcode.ErrServiceUnavailable
		}
		options := f.FieldOptions
		if (f.FieldType == "number" || f.FieldType == "rating") && sameJSONConfig(options, json.RawMessage(`[]`)) {
			options = nil
		}
		inputs = append(inputs, model.TicketTypeFieldInput{
			FieldKey: f.FieldKey, FieldLabel: f.FieldLabel, FieldType: f.FieldType, FieldOptions: options, FieldMeta: f.FieldMeta,
			Required: f.Required, ValidateRegex: f.ValidateRegex, SortOrder: f.SortOrder,
		})
		canonical := *f
		canonical.ID = 0
		snapshotFields[i] = &canonical
	}
	if _, err := validateFieldInputs(inputs); err != nil {
		return "", err
	}
	if err := validatePublishedEvidenceSpec(draft.TransitionMeta, fields); err != nil {
		return "", err
	}
	snap := *draft
	snap.ReadPolicy, snap.ReadRoleCodes = "", nil
	snap.ID, snap.Version, snap.IsActive, snap.CurrentPublishedVersion, snap.CreatedAt = 0, 0, false, nil, time.Time{}
	raw, err := json.Marshal(model.TicketDefinitionSnapshot{Type: snap, Fields: snapshotFields})
	if err != nil {
		return "", err
	}
	return s.ticketRepo.DefinitionSnapshotHashTx(ctx, tx, raw)
}

func validatePublishedEvidenceSpec(raw json.RawMessage, fields []*model.TicketTypeField) error {
	if len(raw) == 0 {
		return nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return errcode.ErrInvalidParams
	}
	editable := make(map[string]bool, len(fields))
	for _, field := range fields {
		var authority struct {
			WriteAuthority string `json:"write_authority"`
		}
		if len(field.FieldMeta) > 0 && json.Unmarshal(field.FieldMeta, &authority) != nil {
			return errcode.ErrInvalidParams
		}
		editable[field.FieldKey] = field.FieldType != "tips" && authority.WriteAuthority == "" && formulaOf(field.FieldMeta) == ""
	}
	for edge, spec := range metadata {
		if !strings.Contains(edge, ">") {
			continue
		}
		collect, err := parseEvidenceCollectFields(spec)
		if err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transition_meta[%s].collect_fields 结构不合法", edge))
		}
		for _, field := range collect {
			if !editable[field.Key] {
				return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transition_meta[%s] 的字段 %q 不可收集", edge, field.Key))
			}
		}
	}
	return nil
}

func (s *Service) GetPublishedTicketForm(ctx context.Context, code string, actor int64) (*model.PublishedTicketForm, error) {
	var result *model.PublishedTicketForm
	err := s.withDefinitionHTTP(ctx, actor, "GET", definitionTypePath+"/"+code+"/published", func(ctx context.Context, tx pgx.Tx, purpose middleware.IAMRequestPurpose) error {
		config, err := s.ticketRepo.HasDefinitionConfigMenu(ctx, actor, false)
		if err != nil {
			return err
		}
		if !config {
			if _, err := s.freshCommandRoles(ctx, actor, purpose.Model, ticketPolicyRoute{path: ticketListPath, method: "POST"}); err != nil {
				return err
			}
		}
		def, err := s.ticketRepo.CurrentDefinitionForForm(ctx, tx, code)
		if err != nil {
			return err
		}
		if def == nil {
			return &DefinitionPublicationError{Reason: "definition_unpublished", Cause: errcode.New(errcode.ErrServiceUnavailable.Code, "工单类型尚未发布")}
		}
		snap, err := decodePublishedDefinition(def)
		if err != nil {
			return err
		}
		t := snap.Type
		result = &model.PublishedTicketForm{
			Type: model.PublishedTicketFormType{Code: t.Code, Name: t.Name, Description: t.Description, States: t.States, Transitions: t.Transitions,
				DefaultSLAHours: t.DefaultSLAHours, HasCustomFields: t.HasCustomFields, IsActive: true},
			Fields:        make([]model.PublishedTicketFormField, 0, len(snap.Fields)),
			DefinitionRef: publicationDefinitionRef(def),
		}
		for _, f := range snap.Fields {
			field, err := projectPublishedTicketField(code, f)
			if err != nil {
				return err
			}
			result.Fields = append(result.Fields, field)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) ListTicketConfigurationTypes(ctx context.Context, actor int64) ([]*model.TicketType, error) {
	var types []*model.TicketType
	err := s.withDefinitionHTTP(ctx, actor, "GET", definitionTypePath, func(ctx context.Context, tx pgx.Tx, _ middleware.IAMRequestPurpose) error {
		config, err := s.ticketRepo.HasDefinitionConfigMenu(ctx, actor, false)
		if err != nil {
			return err
		}
		if config {
			types, err = s.ticketRepo.ListDefinitionDraftTypes(ctx, tx)
		} else {
			types, err = s.ticketRepo.ListPublishedDefinitionTypes(ctx, tx)
		}
		return err
	})
	return types, err
}

func (s *Service) GetTicketDefinitionDraftFields(ctx context.Context, code string, actor int64) ([]*model.TicketTypeField, error) {
	var fields []*model.TicketTypeField
	err := s.withDefinitionHTTP(ctx, actor, "GET", definitionTypePath+"/"+code+"/fields", func(ctx context.Context, tx pgx.Tx, _ middleware.IAMRequestPurpose) error {
		config, err := s.ticketRepo.HasDefinitionConfigMenu(ctx, actor, false)
		if err != nil {
			return err
		}
		if !config {
			return errcode.ErrNoPermission
		}
		if _, err := s.ticketRepo.GetTicketType(ctx, code); err != nil {
			return err
		}
		fields, err = s.ticketRepo.ListTicketTypeFieldsTx(ctx, tx, code)
		return err
	})
	return fields, err
}
