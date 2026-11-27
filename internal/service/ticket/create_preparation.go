package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type preparedTicketCreate struct {
	req      model.CreateTicketRequest
	actor    int64
	ttype    *model.TicketType
	template *model.TicketTemplate
	fields   []*model.TicketTypeField
	ticket   model.Ticket
	humanKey string
}

func cloneCreateTicketRequest(req *model.CreateTicketRequest) model.CreateTicketRequest {
	copy := *req
	copy.CustomData = append(json.RawMessage(nil), req.CustomData...)
	copy.Attachments = append([]string(nil), req.Attachments...)
	if req.EscalationSupervisor != nil {
		value := *req.EscalationSupervisor
		copy.EscalationSupervisor = &value
	}
	if req.AssignedTo != nil {
		value := *req.AssignedTo
		copy.AssignedTo = &value
	}
	return copy
}

// prepareTicketCreate performs provider calls and key allocation before entering
// the transaction. Authorization belongs to the admitting ordinary/risk service.
func (s *Service) prepareTicketCreate(ctx context.Context, input *model.CreateTicketRequest, actorUserID int64) (*preparedTicketCreate, error) {
	frozen := cloneCreateTicketRequest(input)
	req := &frozen
	// 1. 校验 type_code（停用类型对客户端视同不存在，统一 90003）
	ttype, fields, err := s.ticketRepo.PublishedCreateDefinition(ctx, req.TypeCode)
	if err != nil {
		return nil, err
	}
	if !ttype.IsActive {
		return nil, errcode.ErrTicketTypeNotFound
	}
	if req.DefinitionVersion != "" || req.DefinitionHash != "" {
		version, err := strconv.ParseInt(req.DefinitionVersion, 10, 64)
		if err != nil || version <= 0 || strconv.FormatInt(version, 10) != req.DefinitionVersion || len(req.DefinitionHash) != 64 {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, "建单预览须同时传入规范版本与定义摘要")
		}
		if ttype.CurrentPublishedVersion == nil || *ttype.CurrentPublishedVersion != version {
			return nil, errcode.ErrConcurrentModification
		}
		def, err := s.ticketRepo.GetTicketDefinitionTx(ctx, repository.IAMDB(ctx, s.db), req.TypeCode, version)
		if err != nil {
			return nil, err
		}
		if def.DefinitionHash != req.DefinitionHash {
			return nil, errcode.ErrConcurrentModification
		}
	}
	// Historical definitions remain readable and repairable, but must not
	// create a new ticket in a built-in state absent from their own graph.
	if err := validateTypeGraph(ttype.States, ttype.Transitions); err != nil {
		return nil, err
	}

	// 2. 模板预填（可选，09-ticket §2：default_fields 预填空缺字段、default_priority 作缺省）。
	// 取舍：请求显式值 > 模板默认 > 类型缺省 3；模板只填请求未提供的字段，不覆盖显式输入。
	// title 不预填——请求 binding:"required" 保证非空，模板 title 永远不会被用到。
	priority := req.Priority
	if priority == 0 {
		priority = defaultTicketPriority
	}
	// IW1 后随 BK-18 收敛：刻度 1–4（1紧急 2高 3中 4低），越界拒绝（业界枚举口径）
	if priority < 1 || priority > 4 {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "priority 须为 1–4（1紧急 2高 3中 4低）")
	}
	description := req.Description
	customData := req.CustomData
	var template *model.TicketTemplate
	if req.TemplateCode != "" {
		tmpl, err := s.ticketRepo.GetTicketTemplate(ctx, req.TemplateCode)
		switch {
		case errors.Is(err, errcode.ErrNotFound):
			// 模板不存在：可选参数，静默跳过（「命中则预填」语义）
		case err != nil:
			return nil, fmt.Errorf("get ticket template: %w", err) // DB 错误不得吞掉
		default:
			template = tmpl
			if tmpl.TypeCode != req.TypeCode {
				return nil, errcode.ErrInvalidParams // 模板属于其它工单类型
			}
			var defaults struct {
				Description *string          `json:"description,omitempty"`
				CustomData  *json.RawMessage `json:"custom_data,omitempty"`
			}
			if len(tmpl.DefaultFields) > 0 {
				if err := json.Unmarshal(tmpl.DefaultFields, &defaults); err != nil {
					return nil, fmt.Errorf("parse template default_fields: %w", err)
				}
			}
			if description == "" && defaults.Description != nil {
				description = *defaults.Description
			}
			if len(customData) == 0 && defaults.CustomData != nil {
				customData = *defaults.CustomData
			}
			if req.Priority == 0 && tmpl.DefaultPriority > 0 {
				priority = tmpl.DefaultPriority
			}
		}
	}
	// 模板默认优先级路径统一收敛（显式路径已在上方校验）
	if priority < 1 || priority > 4 {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "priority 须为 1–4（1紧急 2高 3中 4低）")
	}

	// IW3/BK-18 G2：按类型字段 schema 校验 custom_data（required/类型/选项/regex）
	// B1-4 取数联动在事务准备阶段、formula 重算之前——取数结果可被
	// formula 引用；失败不阻塞提交、目标字段留空+missed 日志（A.4 拍板）
	// F-B-3（审计 2026-10-22）：fetch 先于校验——fetch 写入的值须过 G2
	// 类型/regex 校验（原顺序 validate→fetch 导致 fetch 结果绕校验）
	// Fetch decisions and the later validation use one exact schema snapshot.
	customData = s.applyFetches(req.TypeCode, customData, actorUserID, fields)
	// S6 §5.3 人读编号事务外预取：nextval 本就非事务性（回滚烧号可接受），
	// 事务内仅保留写行（原子性不变）——keyGen 持池连接，同上禁入事务窗口
	var humanKey string
	if s.keyGen != nil {
		humanKey, err = s.keyGen.Next(ctx)
		if err != nil {
			return nil, fmt.Errorf("human_key: %w", err)
		}
	}

	ticket := model.Ticket{
		TypeCode:             req.TypeCode,
		DefinitionVersion:    ttype.CurrentPublishedVersion,
		Title:                req.Title,
		Description:          description,
		Priority:             priority,
		Status:               StatusOpen,
		CreatedBy:            actorUserID,
		EscalationSupervisor: req.EscalationSupervisor,
		OrgID:                req.OrgID,
		CustomData:           customData,
	}

	return &preparedTicketCreate{req: frozen, actor: actorUserID, ttype: ttype, template: template, fields: fields, ticket: ticket, humanKey: humanKey}, nil
}

// lockPreparedCreateTx takes configuration then org locks, before any risk/ticket
// locks. Risk callers authorize their own approved target in the owning service.
func (s *Service) lockPreparedCreateTx(ctx context.Context, tx pgx.Tx, p *preparedTicketCreate, ordinary bool) (*model.Ticket, error) {
	if err := s.ticketRepo.LockPublishedCreateTx(ctx, tx, p.ttype, p.req.TemplateCode, p.template); err != nil {
		return nil, err
	}
	ticket := p.ticket
	ticket.CustomData = append(json.RawMessage(nil), p.ticket.CustomData...)
	if ticket.DefinitionVersion != nil {
		v := *ticket.DefinitionVersion
		ticket.DefinitionVersion = &v
	}
	if ticket.EscalationSupervisor != nil {
		id := *ticket.EscalationSupervisor
		ticket.EscalationSupervisor = &id
	}
	_, ref, err := s.ticketRepo.BoundTicketDefinitionTx(ctx, tx, ticket.TypeCode, ticket.DefinitionVersion)
	if err != nil {
		return nil, err
	}
	if p.req.DefinitionVersion != "" && (ref == nil || strconv.FormatInt(ref.DefinitionVersion, 10) != p.req.DefinitionVersion || ref.DefinitionHash != p.req.DefinitionHash) {
		return nil, errcode.ErrConcurrentModification
	}
	ticket.DefinitionRef = ref
	ticket.DefinitionResolution = "exact"
	org, err := s.orgRepo.FindByIDForShareTx(ctx, tx, p.req.OrgID)
	if err != nil {
		return nil, err
	}
	ticket.OrgPath = org.Path
	if ordinary {
		ticket.WithdrawalEligible = true
		if err := s.authorizeCreateOrg(ctx, p.actor, p.req.OrgID); err != nil {
			return nil, err
		}
	}
	return &ticket, nil
}

// writePreparedCreateTx preserves the original created/SLA history. The optional
// link writer runs after the ticket insert and before any file locks.
func (s *Service) writePreparedCreateTx(ctx context.Context, tx pgx.Tx, p *preparedTicketCreate, ticket *model.Ticket, link RiskDispatchLinkWriter) error {
	// Validation expands dictionary options in-place; each invocation owns fields.
	fields := make([]*model.TicketTypeField, len(p.fields))
	for i, f := range p.fields {
		copy := *f
		copy.FieldOptions = append(json.RawMessage(nil), f.FieldOptions...)
		fields[i] = &copy
	}
	customData, err := s.validateCustomData(ctx, tx, p.req.TypeCode, ticket.CustomData, nil, fields)
	if err != nil {
		return err
	}
	refs, err := fieldReferences(fields, customData)
	if err != nil {
		return err
	}
	ticket.CustomData = customData
	if err := s.ticketRepo.CreateTx(ctx, tx, ticket); err != nil {
		return err
	}
	if err := s.applyStateSLARuleTx(ctx, tx, p.ttype, ticket, StatusOpen); err != nil {
		return err
	}
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{TicketID: ticket.ID, UserID: p.actor, Action: EventCreated}); err != nil {
		return err
	}
	if link != nil {
		if err := writePreparedTicketHumanKeyTx(ctx, tx, ticket, p.humanKey); err != nil {
			return err
		}
		if err := link(ctx, tx, ticket.ID); err != nil {
			return err
		}
	}
	if err := s.bindUpdateReferencesTx(ctx, tx, ticket.ID, p.actor, p.req.Attachments, refs); err != nil {
		return err
	}
	return writePreparedTicketHumanKeyTx(ctx, tx, ticket, p.humanKey)
}

func writePreparedTicketHumanKeyTx(ctx context.Context, tx pgx.Tx, ticket *model.Ticket, key string) error {
	if ticket.HumanKey == "" && key != "" {
		if _, err := tx.Exec(ctx, `UPDATE tickets SET human_key = $1 WHERE id = $2`, key, ticket.ID); err != nil {
			return fmt.Errorf("write human_key: %w", err)
		}
		ticket.HumanKey = key
	}
	return nil
}
