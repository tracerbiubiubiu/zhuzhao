package ticket

// IW3/BK-18：工单类型/字段/模板管理（写侧服务层）
// SSOT: docs/phase2/00 §9 BK-18、docs/phase3/12-frontend §2

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/exprcalc"
)

// 000010 默认状态图（新类型缺省；与 DDL DEFAULT 保持一致）
const (
	defaultStates      = `["open","assigned","in_progress","pending_verify","closed","rejected"]`
	defaultTransitions = `{"open":["assigned","closed"],"assigned":["in_progress","open"],"in_progress":["pending_verify","rejected","closed"],"pending_verify":["closed","in_progress"],"closed":["open"],"rejected":["open"]}`
)

// 允许的字段类型（8 种，12-frontend §3.1 DynamicFormField 同构；richtext 第八种=B5-4，
// 05 号 §6——存储态 MD 源文本、服务端白名单渲染为 SSOT，只增不改）
var validFieldTypes = map[string]bool{
	"input": true, "textarea": true, "number": true, "date": true,
	"select": true, "multi_select": true, "tips": true, "richtext": true,
	// B1-6 批1（第 9/10/11 类型）：radio=与 select 同数据模型不同渲染；
	// rating=与 number 同模型；file=上传字段（object_key 数组——01 号 §2/§3.5）
	"radio": true, "rating": true, "file": true,
}

// RichtextMaxBytes richtext 字段 MD 源文本上限（防 custom_data JSONB 慢性膨胀——图片走对象存储
// 稳定 URL、禁 base64 内联为既定纪律，正文 256 KiB 远超工单叙述需求）；可随 B5-6 限制配置化入 KV。
// 导出供渲染端点同限（入站校验与渲染入口一致）。
const RichtextMaxBytes = 256 << 10

// CreateTicketType 新建类型（states/transitions 缺省默认图）
func (s *Service) CreateTicketType(ctx context.Context, req *model.CreateTicketTypeRequest) (*model.TicketType, error) {
	states := json.RawMessage(req.States)
	if len(states) == 0 {
		states = json.RawMessage(defaultStates)
	}
	transitions := json.RawMessage(req.Transitions)
	if len(transitions) == 0 {
		transitions = json.RawMessage(defaultTransitions)
	}
	if err := validateTypeGraph(states, transitions); err != nil {
		return nil, err
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	t := &model.TicketType{
		Code: req.Code, Name: req.Name, Description: req.Description,
		States: states, Transitions: transitions, IsActive: isActive,
	}
	if err := s.ticketRepo.CreateTicketType(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTicketType 更新类型（patch；code 不可改）。
// P1-3：req.Version 可选乐观锁——传了才校验（nil = 旧行为）。
func (s *Service) UpdateTicketType(ctx context.Context, code string, req *model.UpdateTicketTypeRequest) (*model.TicketType, error) {
	if req.Version != nil && *req.Version <= 0 {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "version 须为正整数")
	}
	if len(req.States) > 0 || len(req.Transitions) > 0 {
		states, transitions := req.States, req.Transitions
		if len(states) == 0 {
			states = json.RawMessage(defaultStates)
		}
		if len(transitions) == 0 {
			transitions = json.RawMessage(defaultTransitions)
		}
		if err := validateTypeGraph(states, transitions); err != nil {
			return nil, err
		}
	}
	// B10-4：transition_meta 键域须 ⊆ states（改图后残留旧状态键 → 400 收口，
	// 防扫描器扫已删状态的悬空超时配置）。仅发 meta 时取存量 states 判键域。
	if len(req.TransitionMeta) > 0 {
		stateList, t, err := s.metaKeyDomain(ctx, code, req.States)
		if err != nil {
			return nil, err
		}
		if err := validateTransitionMeta(stateList, req.TransitionMeta); err != nil {
			return nil, err
		}
		// S3a B3-1："from>to" 转换级键校验（条件 roles 词表+validator 语法+键域
		// ⊆ transitions 展开集——与 B10-4 state 级键共存同列两形）
		var transitions map[string][]string
		if len(req.Transitions) > 0 {
			_ = json.Unmarshal(req.Transitions, &transitions)
		} else {
			_ = json.Unmarshal(t.Transitions, &transitions)
		}
		if err := ValidateTransitionMeta(stateList, transitions, req.TransitionMeta); err != nil {
			return nil, err
		}
	}
	return s.ticketRepo.UpdateTicketType(ctx, code, req.Name, req.Description, req.States, req.Transitions, req.TransitionMeta, req.IsActive, req.Version)
}

// metaKeyDomain 取 meta 键域判定的 states+transitions 基线：请求带 states 用请求值，
// 否则读存量。返回 (states, 存量类型对象供 transitions 读取)。
func (s *Service) metaKeyDomain(ctx context.Context, code string, reqStates json.RawMessage) ([]string, *model.TicketType, error) {
	t, err := s.ticketRepo.GetTicketType(ctx, code)
	if err != nil {
		return nil, nil, err
	}
	var stateList []string
	if len(reqStates) > 0 {
		if err := json.Unmarshal(reqStates, &stateList); err != nil {
			return nil, nil, errcode.New(errcode.ErrInvalidParams.Code, "states 须为字符串数组")
		}
		return stateList, t, nil
	}
	if err := json.Unmarshal(t.States, &stateList); err != nil {
		return nil, nil, errcode.New(errcode.ErrInvalidParams.Code, "存量 states 解析失败")
	}
	return stateList, t, nil
}

// validateTransitionMeta B10-4：state 级节点元数据结构校验（键⊆states；值含
// timeout_minutes>0 与 escalation 可选嵌套段——notify⊆{assignee,supervisor}、
// reminder_interval_minutes/max_reminders>0）。零 DB 纯函数（单测直测）。
func validateTransitionMeta(states []string, meta json.RawMessage) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(meta, &m); err != nil {
		return errcode.New(errcode.ErrInvalidParams.Code, "transition_meta 须为对象")
	}
	set := map[string]bool{}
	for _, st := range states {
		set[st] = true
	}
	for state, raw := range m {
		if !set[state] {
			if strings.Contains(state, ">") {
				continue // "from>to" 转换级键（S3a B3-1）——归 ValidateTransitionMeta 校验
			}
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transition_meta 键 %q 不在 states 中", state))
		}
		var seg struct {
			TimeoutMinutes *int `json:"timeout_minutes"`
			Escalation     *struct {
				Notify                  []string `json:"notify"`
				ReminderIntervalMinutes *int     `json:"reminder_interval_minutes"`
				MaxReminders            *int     `json:"max_reminders"`
			} `json:"escalation"`
		}
		if err := json.Unmarshal(raw, &seg); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transition_meta[%q] 结构不合法", state))
		}
		if seg.TimeoutMinutes != nil && *seg.TimeoutMinutes <= 0 {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transition_meta[%q].timeout_minutes 须为正整数", state))
		}
		if e := seg.Escalation; e != nil {
			for _, n := range e.Notify {
				if n != "assignee" && n != "supervisor" {
					return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transition_meta[%q].escalation.notify 含未知目标 %q（assignee/supervisor）", state, n))
				}
			}
			// 下限 5 分钟：S6 通知链有 60s 同单同动作节流窗（06 号 §5.2 投递语义），
			// 催办间隔若小于节流窗会被吞——校验层挡在配置面而非发送面
			if e.ReminderIntervalMinutes != nil && *e.ReminderIntervalMinutes < 5 {
				return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transition_meta[%q].escalation.reminder_interval_minutes 须 ≥5 分钟（小于通知节流窗会被吞）", state))
			}
			if e.MaxReminders != nil && *e.MaxReminders < 0 {
				return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transition_meta[%q].escalation.max_reminders 须为非负整数", state))
			}
		}
	}
	return nil
}

// DeleteTicketType 删除类型（有工单禁删 → ErrConflict，走停用）
func (s *Service) DeleteTicketType(ctx context.Context, code string) error {
	return s.ticketRepo.DeleteTicketType(ctx, code)
}

// ListTicketTypesFor 列表入口：includeInactive 仅 admin/superadmin 生效（否则忽略该参数）
func (s *Service) ListTicketTypesFor(ctx context.Context, actorUserID int64, includeInactive bool) ([]*model.TicketType, error) {
	if !includeInactive {
		return s.ticketRepo.ListTicketTypes(ctx)
	}
	roles, err := s.getRoles(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if HasRole(roles, "admin") || HasRole(roles, "superadmin") {
		return s.ticketRepo.ListTicketTypesAdmin(ctx)
	}
	return s.ticketRepo.ListTicketTypes(ctx)
}

// ListTicketTypeFieldsAdmin 管理端字段读取（含 validate_regex）
func (s *Service) ListTicketTypeFieldsAdmin(ctx context.Context, typeCode string) ([]*model.TicketTypeField, error) {
	if _, err := s.ticketRepo.GetTicketType(ctx, typeCode); err != nil {
		return nil, err
	}
	return s.ticketRepo.ListTicketTypeFieldsAll(ctx, typeCode)
}

// FieldSoftWarning 锁死守卫软提示（保存可继续——在途计数提醒；A.5 2026-10-22
// 拍板：软提示=options 收窄/required false→true，借 impact 在途计数）。
type FieldSoftWarning struct {
	Field    string `json:"field"`
	Reason   string `json:"reason"`
	InFlight int    `json:"in_flight"`
}

// ReplaceTicketTypeFields 全量替换字段集（校验：key 唯一/类型枚举/select 选项/正则可编译）。
// P1-3：req.Version 可选乐观锁，以父类型 ticket_types.version 做 CAS；nil = 旧行为。
func (s *Service) ReplaceTicketTypeFields(ctx context.Context, code string, req *model.ReplaceTypeFieldsRequest) ([]FieldSoftWarning, error) {
	if req.Version != nil && *req.Version <= 0 {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "version 须为正整数")
	}
	if _, err := s.ticketRepo.GetTicketType(ctx, code); err != nil {
		return nil, err
	}
	fields, err := validateFieldInputs(req.Fields)
	if err != nil {
		return nil, err
	}
	// B1-7 批2 锁死守卫（01 号 §2/A.5 拍板终版——兼容性判据版）：硬锁 400 =
	// 改 field_type / 改 field_key / 删除（按 key diff 新旧，判据=是否破坏已落库
	// 数据或旧单编辑）；软提示（可继续，借 impact 在途计数）= options 收窄 /
	// required false→true；余自由（label/排序/新增/formula/fetch/validation/
	// options 只增不减/required true→false）。首次定义（旧集空）不受锁。
	old, err := s.ticketRepo.ListTicketTypeFieldsAll(ctx, code)
	if err != nil {
		return nil, err
	}
	if err := checkFieldLock(old, fields); err != nil {
		return nil, err
	}
	if err := s.checkDictCodesExist(ctx, fields); err != nil {
		return nil, err
	}
	warnings := s.softFieldWarnings(ctx, code, old, fields)
	return warnings, s.ticketRepo.ReplaceTypeFields(ctx, code, fields, req.Version)
}

// checkFieldLock 锁死守卫硬锁面（纯函数便于测试）：旧集非空时，旧 key 必须全部
// 存在于新集（删/改 key 均表现为旧 key 缺失）且 field_type 不可变。
func checkFieldLock(old []*model.TicketTypeField, neu []model.TicketTypeField) error {
	if len(old) == 0 {
		return nil
	}
	newByKey := make(map[string]*model.TicketTypeField, len(neu))
	for i := range neu {
		newByKey[neu[i].FieldKey] = &neu[i]
	}
	for _, o := range old {
		n, ok := newByKey[o.FieldKey]
		if !ok {
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("字段 %q 已有存量数据引用，删除/改 key 属硬锁不可达——锁死守卫（01 号 §2/A.5）", o.FieldKey))
		}
		if n.FieldType != o.FieldType {
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("字段 %q 的 field_type 不可变更（%s → %s）——锁死守卫；如需换型请新建字段", o.FieldKey, o.FieldType, n.FieldType))
		}
	}
	return nil
}

// checkDictCodesExist B1-7 批2（三路F-B-8 收口）：字典引用保存期存在性校验——
// 码不存在的字典在运行期 expandDictOptions 展开为空集（校验跳过=旁路），
// 保存期前置拦截。只查存在不查启用：运行期按 dict_items.enabled 展开（类型
// 停用不构成旁路），与 expandDictOptions 语义对齐。
func (s *Service) checkDictCodesExist(ctx context.Context, fields []model.TicketTypeField) error {
	for i := range fields {
		f := &fields[i]
		if f.FieldType != "select" && f.FieldType != "multi_select" && f.FieldType != "radio" {
			continue
		}
		var elems []json.RawMessage
		if json.Unmarshal(f.FieldOptions, &elems) != nil || len(elems) != 1 {
			continue // 非单元素字典引用形态（具体选项列表）不查
		}
		var sv string
		if json.Unmarshal(elems[0], &sv) != nil || !strings.HasPrefix(sv, "dict:") {
			continue
		}
		code := strings.TrimPrefix(sv, "dict:")
		var exists bool
		if err := s.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM dict_types WHERE code = $1)`, code).Scan(&exists); err != nil {
			return errcode.ErrInternal
		}
		if !exists {
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("字段 %q 引用的字典 %q 不存在——保存被拒（运行期空集=校验旁路）", f.FieldKey, code))
		}
	}
	return nil
}

// softFieldWarnings 软提示集（不阻断）：options 收窄 / required false→true，
// 附 FieldImpact 在途计数；计数查询失败静默降级为无计数提示（软语义不阻塞）。
func (s *Service) softFieldWarnings(ctx context.Context, code string, old []*model.TicketTypeField, neu []model.TicketTypeField) []FieldSoftWarning {
	if len(old) == 0 {
		return nil
	}
	newByKey := make(map[string]*model.TicketTypeField, len(neu))
	for i := range neu {
		newByKey[neu[i].FieldKey] = &neu[i]
	}
	type softRef struct{ field, reason string }
	var softs []softRef
	for _, o := range old {
		n, ok := newByKey[o.FieldKey]
		if !ok {
			continue // 硬锁已拦，此处不可达
		}
		if !o.Required && n.Required {
			softs = append(softs, softRef{o.FieldKey, "required 加严（false→true）——在途单编辑将按必填校验"})
		}
		if (o.FieldType == "select" || o.FieldType == "multi_select") && o.FieldType == n.FieldType {
			oldVals, err1 := optionValues(o.FieldOptions)
			newVals, err2 := optionValues(n.FieldOptions)
			if err1 == nil && err2 == nil && len(oldVals) > len(newVals) {
				nv := map[string]bool{}
				for _, v := range newVals {
					nv[v] = true
				}
				narrowed := false
				for _, v := range oldVals {
					if !nv[v] {
						narrowed = true
						break
					}
				}
				if narrowed {
					softs = append(softs, softRef{o.FieldKey, "options 收窄——在途单中的旧取值编辑时将不符新选项集"})
				}
			}
		}
	}
	if len(softs) == 0 {
		return nil
	}
	impact, err := s.FieldImpact(ctx, code)
	if err != nil {
		impact = nil // 软语义：计数失败降级为无计数
	}
	ws := make([]FieldSoftWarning, 0, len(softs))
	for _, sr := range softs {
		ws = append(ws, FieldSoftWarning{Field: sr.field, Reason: sr.reason, InFlight: impact[sr.field]})
	}
	return ws
}

// CreateTicketTemplate 新建模模板（type 须存在；org 须存在，org_path 服务端解析）
func (s *Service) CreateTicketTemplate(ctx context.Context, req *model.CreateTicketTemplateRequest, actorUserID int64) (*model.TicketTemplate, error) {
	if _, err := s.ticketRepo.GetTicketType(ctx, req.TypeCode); err != nil {
		return nil, err
	}
	if req.DefaultPriority != 0 && (req.DefaultPriority < 1 || req.DefaultPriority > 4) {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "default_priority 须为 1–4")
	}
	org, err := s.orgRepo.FindByID(ctx, req.OrgID)
	if err != nil {
		return nil, err
	}
	t := &model.TicketTemplate{
		Code: req.Code, Name: req.Name, TypeCode: req.TypeCode,
		DefaultPriority: req.DefaultPriority, DefaultFields: req.DefaultFields,
		DefaultSLAMinutes: req.DefaultSLAMinutes,
		OrgID:             org.ID, OrgPath: org.Path, CreatedBy: actorUserID,
	}
	if err := s.ticketRepo.CreateTicketTemplate(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTicketTemplate 更新模板（patch；code/type/org 不可改）。
// P1-3：req.Version 可选乐观锁——传了才校验（nil = 旧行为）。
func (s *Service) UpdateTicketTemplate(ctx context.Context, code string, req *model.UpdateTicketTemplateRequest) (*model.TicketTemplate, error) {
	if req.Version != nil && *req.Version <= 0 {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "version 须为正整数")
	}
	if req.DefaultPriority != nil && (*req.DefaultPriority < 1 || *req.DefaultPriority > 4) {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "default_priority 须为 1–4")
	}
	return s.ticketRepo.UpdateTicketTemplate(ctx, code, req.Name, req.DefaultPriority, req.DefaultFields, req.DefaultSLAMinutes, req.Version)
}

// DeleteTicketTemplate 删除模板
func (s *Service) DeleteTicketTemplate(ctx context.Context, code string) error {
	return s.ticketRepo.DeleteTicketTemplate(ctx, code)
}

// validateTypeGraph states 必须为非空字符串数组；transitions 必须为
// {from: [to...]} 且端点均在 states 内
func validateTypeGraph(states, transitions json.RawMessage) error {
	var stateList []string
	if err := json.Unmarshal(states, &stateList); err != nil {
		return errcode.New(errcode.ErrInvalidParams.Code, "states 须为字符串数组")
	}
	set := map[string]bool{}
	for _, st := range stateList {
		set[st] = true
	}
	if len(set) == 0 {
		return errcode.New(errcode.ErrInvalidParams.Code, "states 不能为空")
	}
	var trans map[string][]string
	if err := json.Unmarshal(transitions, &trans); err != nil {
		return errcode.New(errcode.ErrInvalidParams.Code, "transitions 须为 {from: [to...]} 对象")
	}
	for from, tos := range trans {
		if !set[from] {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transitions.from %q 不在 states 中", from))
		}
		for _, to := range tos {
			if !set[to] {
				return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("transitions.to %q 不在 states 中", to))
			}
		}
	}
	return nil
}

// validateFieldInputs 字段输入校验；返回带排序/选项规整的领域字段列表
func validateFieldInputs(inputs []model.TicketTypeFieldInput) ([]model.TicketTypeField, error) {
	seen := map[string]bool{}
	fields := make([]model.TicketTypeField, 0, len(inputs))
	for i, in := range inputs {
		if seen[in.FieldKey] {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("field_key %q 重复", in.FieldKey))
		}
		seen[in.FieldKey] = true
		if !validFieldTypes[in.FieldType] {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("field_type %q 不支持", in.FieldType))
		}
		f := model.TicketTypeField{
			FieldKey: in.FieldKey, FieldLabel: in.FieldLabel, FieldType: in.FieldType,
			FieldOptions: in.FieldOptions, FieldMeta: in.FieldMeta, Required: in.Required,
			ValidateRegex: in.ValidateRegex, SortOrder: in.SortOrder,
		}
		if f.SortOrder == 0 {
			f.SortOrder = (i + 1) * 10
		}
		// B1-3 formula 语法预检：field_meta.formula 非空时编译校验（附录 A.3——
		// 保存时即拒非法表达式；编译缓存零成本）。formula 只允许 number/input 型。
		if len(in.FieldMeta) > 0 {
			var meta struct {
				Formula string `json:"formula"`
			}
			if err := json.Unmarshal(in.FieldMeta, &meta); err != nil {
				return nil, errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("field_key %q 的 field_meta 须为 JSON 对象", in.FieldKey))
			}
			if meta.Formula != "" {
				if in.FieldType != "number" && in.FieldType != "input" {
					return nil, errcode.New(errcode.ErrInvalidParams.Code,
						fmt.Sprintf("field_key %q：formula 仅允许 number/input 型字段", in.FieldKey))
				}
				if err := exprcalc.CheckSyntax(meta.Formula); err != nil {
					return nil, errcode.New(errcode.ErrInvalidParams.Code,
						fmt.Sprintf("field_key %q 的 formula 语法错误: %v", in.FieldKey, err))
				}
			}
		}
		// select/multi_select/radio 选项规整：接受 ["a","b"] 或 [{"label","value"}]；空选项拒绝
		// （B1-6 批1：radio 与 select 同数据模型——G2/字典引用共用校验）
		if in.FieldType == "select" || in.FieldType == "multi_select" || in.FieldType == "radio" {
			values, err := optionValues(in.FieldOptions)
			if err != nil || len(values) == 0 {
				return nil, errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("字段 %q 为 select/multi_select，field_options 须为非空选项数组", in.FieldKey))
			}
		}
		if (in.FieldType == "number" || in.FieldType == "rating") && len(in.FieldOptions) > 0 {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 为 %s，不接受 field_options", in.FieldKey, in.FieldType))
		}
		// B1-6 批1：file 的 field_options = 可选 MIME 白名单（字符串数组，元素含 "/"）
		if in.FieldType == "file" && len(in.FieldOptions) > 0 {
			var mimes []string
			if err := json.Unmarshal(in.FieldOptions, &mimes); err != nil {
				return nil, errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 为 file，field_options 须为 MIME 字符串数组", in.FieldKey))
			}
			for _, m := range mimes {
				if !strings.Contains(m, "/") {
					return nil, errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 的 MIME 白名单元素 %q 须形如 image/png", in.FieldKey, m))
				}
			}
		}
		// B1-6 批1：field_meta 布局键校验（col_span 1–24 / group 字符串 / visible_when 语法）
		if len(in.FieldMeta) > 0 {
			var layout struct {
				ColSpan     *int   `json:"col_span"`
				Group       string `json:"group"`
				VisibleWhen string `json:"visible_when"`
			}
			if err := json.Unmarshal(in.FieldMeta, &layout); err == nil {
				if layout.ColSpan != nil && (*layout.ColSpan < 1 || *layout.ColSpan > 24) {
					return nil, errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 的 col_span 须为 1–24", in.FieldKey))
				}
				if layout.VisibleWhen != "" {
					if err := exprcalc.CheckSyntax(layout.VisibleWhen); err != nil {
						return nil, errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 的 visible_when 语法错误: %v", in.FieldKey, err))
					}
				}
			}
		}
		if in.ValidateRegex != "" {
			if _, err := regexp.Compile(in.ValidateRegex); err != nil {
				return nil, errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("字段 %q 的 validate_regex 编译失败: %v", in.FieldKey, err))
			}
		}
		if f.FieldOptions == nil {
			f.FieldOptions = json.RawMessage("[]")
		}
		fields = append(fields, f)
	}
	// B1-7 批2 保存校验三件（A.5——走查定稿拍板）：①禁链式+引用存在性
	// （formula 标识符须为同列表存在的非 formula 数据字段——N1 终局）②fetch
	// 引用（trigger_by 存在且非 formula 字段、mapping 目标存在）③触发引用图
	// 无环（自触发边忽略——trigger_by 缺省即自身）。
	if err := checkFieldReferences(fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// formulaOf 取 field_meta.formula（无则空串）。
func formulaOf(meta json.RawMessage) string {
	if len(meta) == 0 {
		return ""
	}
	var m struct {
		Formula string `json:"formula"`
	}
	_ = json.Unmarshal(meta, &m)
	return m.Formula
}

// fetchConfigOf 取 field_meta.fetch 投影（无 fetch 配置时 has=false）。
func fetchConfigOf(meta json.RawMessage) (triggerBy string, mapping map[string]string, has bool) {
	if len(meta) == 0 {
		return "", nil, false
	}
	var m struct {
		Fetch struct {
			Source struct {
				Type string `json:"type"`
			} `json:"source"`
			TriggerBy string            `json:"trigger_by"`
			Mapping   map[string]string `json:"mapping"`
		} `json:"fetch"`
	}
	if json.Unmarshal(meta, &m) != nil || m.Fetch.Source.Type == "" {
		return "", nil, false
	}
	return m.Fetch.TriggerBy, m.Fetch.Mapping, true
}

// visibleWhenOf 取 field_meta.visible_when（无则空串——B1-6 批1 条件显隐）。
func visibleWhenOf(meta json.RawMessage) string {
	if len(meta) == 0 {
		return ""
	}
	var m struct {
		VisibleWhen string `json:"visible_when"`
	}
	_ = json.Unmarshal(meta, &m)
	return m.VisibleWhen
}

// checkFieldReferences 保存期跨字段引用校验（纯函数）。
func checkFieldReferences(fields []model.TicketTypeField) error {
	byKey := make(map[string]*model.TicketTypeField, len(fields))
	formulaKeys := map[string]bool{}
	for i := range fields {
		byKey[fields[i].FieldKey] = &fields[i]
		if formulaOf(fields[i].FieldMeta) != "" {
			formulaKeys[fields[i].FieldKey] = true
		}
	}
	for i := range fields {
		f := &fields[i]
		if formula := formulaOf(f.FieldMeta); formula != "" {
			ids, err := exprcalc.Identifiers(formula)
			if err != nil {
				return errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("字段 %q 的 formula 解析失败: %v", f.FieldKey, err))
			}
			for _, id := range ids {
				if _, ok := byKey[id]; !ok {
					return errcode.New(errcode.ErrInvalidParams.Code,
						fmt.Sprintf("formula 字段 %q 引用了不存在的字段 %q——引用须为同 schema 数据字段", f.FieldKey, id))
				}
				if formulaKeys[id] {
					return errcode.New(errcode.ErrInvalidParams.Code,
						fmt.Sprintf("formula 字段 %q 引用了 formula 字段 %q——禁止 formula→formula 链式（N1 终局；组合能力归注册函数与 fetcher 接力）", f.FieldKey, id))
				}
			}
		}
		// B1-6 批1：visible_when 引用存在性（引用 formula 字段允许——快照语义与
		// A.3 级联注一致；不存在即保存期拼错）
		if vw := visibleWhenOf(f.FieldMeta); vw != "" {
			ids, err := exprcalc.Identifiers(vw)
			if err == nil {
				for _, id := range ids {
					if _, ok := byKey[id]; !ok {
						return errcode.New(errcode.ErrInvalidParams.Code,
							fmt.Sprintf("字段 %q 的 visible_when 引用了不存在的字段 %q", f.FieldKey, id))
					}
				}
			}
		}
		triggerBy, mapping, has := fetchConfigOf(f.FieldMeta)
		if !has {
			continue
		}
		trigger := triggerBy
		if trigger == "" {
			trigger = f.FieldKey // 缺省自触发（applyFetches 同款默认）
		}
		if _, ok := byKey[trigger]; !ok {
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("字段 %q 的取数触发字段 %q 不存在", f.FieldKey, trigger))
		}
		if formulaKeys[trigger] {
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("字段 %q 的取数触发字段 %q 是 formula 字段——时序不可能（链=取数→校验→公式）", f.FieldKey, trigger))
		}
		for src, dst := range mapping {
			if _, ok := byKey[dst]; !ok {
				return errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("字段 %q 的取数映射 %q→%q 目标字段不存在", f.FieldKey, src, dst))
			}
		}
	}
	return checkTriggerCycle(fields)
}

// checkTriggerCycle 触发引用图无环（边=trigger→取数目标；自环=缺省自触发，忽略）。
func checkTriggerCycle(fields []model.TicketTypeField) error {
	adj := map[string][]string{}
	for i := range fields {
		triggerBy, _, has := fetchConfigOf(fields[i].FieldMeta)
		if !has {
			continue
		}
		trigger := triggerBy
		if trigger == "" {
			trigger = fields[i].FieldKey
		}
		if trigger != fields[i].FieldKey {
			adj[trigger] = append(adj[trigger], fields[i].FieldKey)
		}
	}
	const (
		white, gray, black = 0, 1, 2
	)
	state := map[string]int{}
	var cycleAt string
	var dfs func(string) bool
	dfs = func(n string) bool {
		state[n] = gray
		for _, m := range adj[n] {
			if state[m] == gray {
				cycleAt = n + "↔" + m
				return true
			}
			if state[m] == white && dfs(m) {
				return true
			}
		}
		state[n] = black
		return false
	}
	for n := range adj {
		if state[n] == white && dfs(n) {
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("取数触发引用图成环（%s）——环形接力保存被拒", cycleAt))
		}
	}
	return nil
}

// optionValues 抽取 select/multi_select 的 value 集合（兼容字符串数组与 {label,value} 对象数组）
func optionValues(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, err
	}
	values := make([]string, 0, len(elems))
	for _, e := range elems {
		var sv string
		if err := json.Unmarshal(e, &sv); err == nil {
			values = append(values, sv)
			continue
		}
		var ov struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(e, &ov); err != nil || ov.Value == "" {
			return nil, fmt.Errorf("option 元素须为字符串或 {value}")
		}
		values = append(values, ov.Value)
	}
	return values, nil
}

// CustomDataMaxBytes custom_data JSONB 总量上限（拍板 P5 2026-10-21）：
// 未知键放行（向前兼容——白名单化会令带杂键的存量单无法 patch 保存），
// 但整体体量封顶防跨请求累加膨胀（TOAST/备份放大）；与 RichtextMaxBytes 同族。
const CustomDataMaxBytes = 64 << 10

// expandDictOptions 将 field_options 中的 "dict:<code>" 引用展开为该字典
// 启用项的 code 值集（F-B-2 修复——select 校验须对真实字典项校验而非字面量）。
// 非 dict 引用原样返回；字典码不存在与 DB 查询故障两臂均返回空集 []
// （fail-open——校验侧 len==0 即跳过，"无选项不校验"语义。P1-1 修复
// 2026-10-22：错误臂原返回原文，经 optionValues 成字面量单选项 → 一切
// 合法 select 值被 400 拒，fail-closed 实效与 fail-open 意图相反；码不
// 存在臂由保存期字典码存在性校验收口[B1-7]，运行期空集仅为兜底）。
func (s *Service) expandDictOptions(ctx context.Context, raw json.RawMessage) json.RawMessage {
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil || len(elems) != 1 {
		return raw
	}
	var sv string
	if err := json.Unmarshal(elems[0], &sv); err != nil || !strings.HasPrefix(sv, "dict:") {
		return raw
	}
	code := strings.TrimPrefix(sv, "dict:")
	rows, err := s.db.Query(ctx,
		`SELECT code FROM dict_items WHERE type_code = $1 AND enabled = true ORDER BY sort_order`, code)
	if err != nil {
		return json.RawMessage("[]") // DB 故障→空集=真 fail-open（原文返回会拒一切合法值）
	}
	defer rows.Close()
	codes := make([]string, 0, 8)
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return json.RawMessage("[]")
		}
		codes = append(codes, c)
	}
	b, err := json.Marshal(codes)
	if err != nil {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}

// validateCustomData G2：按类型字段 schema 校验 custom_data（required/类型/regex）。
// 仅校验 schema 已知键（未知键放行，向前兼容）；模板预填发生在合并后的 customData 上。
// validateCustomData G2：按类型字段 schema 校验 custom_data（required/类型/regex）。
// B1-6 批1 起返回剔除隐藏字段后的 custom_data（visible_when=false 的字段 required
// 不校验、值剔除不写入——防绕过；无剔除时原样返回零重编码）。
func (s *Service) validateCustomData(ctx context.Context, typeCode string, customData json.RawMessage) (json.RawMessage, error) {
	// 总量封顶先于 schema 查询（无字段定义的类型同样受限——拍板 P5）
	if len(customData) > CustomDataMaxBytes {
		return nil, errcode.New(errcode.ErrInvalidParams.Code,
			fmt.Sprintf("custom_data 超过 %d KiB 上限", CustomDataMaxBytes>>10))
	}
	fields, err := s.ticketRepo.ListTicketTypeFields(ctx, typeCode)
	if err != nil || len(fields) == 0 {
		return customData, err // 无字段定义（或读取失败）不校验——读取失败随 Create 一起失败
	}
	if len(customData) == 0 {
		customData = json.RawMessage("{}")
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(customData, &data); err != nil {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "custom_data 须为 JSON 对象")
	}
	// B1-6 批1 条件显隐（01 号 §5 翻案）：G2 提交时跳过隐藏字段。求值 env=提交
	// 快照值；求值失败/非布尔结果视为可见（保守=校验更严，不静默放行隐藏值）。
	hidden := map[string]bool{}
	var envData map[string]any
	for _, f := range fields {
		vw := visibleWhenOf(f.FieldMeta)
		if vw == "" {
			continue
		}
		if envData == nil {
			envData = map[string]any{}
			_ = json.Unmarshal(customData, &envData)
		}
		v, err := exprcalc.EvalFormula(vw, exprcalc.EvalEnv{Fields: envData})
		if err != nil {
			continue
		}
		if b, ok := v.(bool); ok && !b {
			hidden[f.FieldKey] = true
		}
	}
	for _, f := range fields {
		if hidden[f.FieldKey] {
			continue // required 不校验 + 值剔除（回写侧统一 prune）
		}
		// F-B-2 外审 P0 修复（2026-10-22）：field_options 为 dict: 引用时展开为
		// 真实字典项值集（optionValues 只见 ["dict:code"] 字面量，真实值全被拒）
		if f.FieldOptions != nil && (f.FieldType == "select" || f.FieldType == "multi_select" || f.FieldType == "radio") {
			f.FieldOptions = s.expandDictOptions(ctx, f.FieldOptions)
		}
		// B1-6 批1 file：required→至少 1 个 object_key（缺失/null/空数组均拒）
		if f.FieldType == "file" && f.Required {
			rawF, presentF := data[f.FieldKey]
			if !presentF || len(rawF) == 0 || string(rawF) == "null" || string(rawF) == "[]" {
				return nil, errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("字段 %q（%s）为必填（至少 1 个文件）", f.FieldKey, f.FieldLabel))
			}
		}
		// B1-6 批1 file：MIME 白名单对照（field_options 列表 × file_objects.mime_type
		// ——presign 钉入的声明锚点，G2 不可凭 object_key 后缀推断）
		if f.FieldType == "file" {
			var allow []string
			_ = json.Unmarshal(f.FieldOptions, &allow)
			if len(allow) > 0 {
				var keys []string
				if rawF, ok := data[f.FieldKey]; ok {
					_ = json.Unmarshal(rawF, &keys)
				}
				if len(keys) > 0 {
					var mimes []string
					if err := s.db.QueryRow(ctx,
						`SELECT array_agg(fo.mime_type) FROM file_objects fo WHERE fo.object_key = ANY($1)`, keys).Scan(&mimes); err != nil {
						return nil, errcode.ErrInternal
					}
					allowSet := map[string]bool{}
					for _, m := range allow {
						allowSet[m] = true
					}
					for _, m := range mimes {
						if m != "" && !allowSet[m] {
							return nil, errcode.New(errcode.ErrInvalidParams.Code,
								fmt.Sprintf("字段 %q 的文件类型 %q 不在白名单内", f.FieldKey, m))
						}
					}
				}
			}
		}
		raw, present := data[f.FieldKey]
		isEmpty := !present || len(raw) == 0 || string(raw) == "null" || string(raw) == `""`
		// F-B-4 外审 P1 修复（2026-10-22）：formula 字段跳过 required 校验——
		// 服务端 applyFormulas 会重算写入值，required 检查在空值时误拒
		//（修复前：required formula 字段 + 用户不填 → 400"必填" → 类型死锁）
		isFormula := false
		if len(f.FieldMeta) > 0 {
			var m struct {
				Formula string `json:"formula"`
			}
			_ = json.Unmarshal(f.FieldMeta, &m)
			isFormula = m.Formula != ""
		}
		if f.Required && isEmpty && !isFormula {
			return nil, errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("字段 %q（%s）为必填", f.FieldKey, f.FieldLabel))
		}
		// 空值跳过类型校验（formula 字段空值亦然——required 已豁免，值由
		// applyFormulas 算入；P2-6 清理 2026-10-22：原三重连判第三支不可达）
		if isEmpty {
			continue
		}
		if err := validateFieldValue(f, raw); err != nil {
			return nil, err
		}
	}
	if len(hidden) > 0 {
		for k := range hidden {
			delete(data, k)
		}
		b, err := json.Marshal(data)
		if err != nil {
			return nil, errcode.ErrInternal
		}
		return json.RawMessage(b), nil
	}
	return customData, nil
}

// validateFieldValue 单字段类型/选项/正则校验
func validateFieldValue(f *model.TicketTypeField, raw json.RawMessage) error {
	switch f.FieldType {
	case "input", "textarea":
		var sv string
		if err := json.Unmarshal(raw, &sv); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 须为字符串", f.FieldKey))
		}
	case "richtext":
		// MD 源文本（存储态纯文本——05 号 §6）；渲染/消毒在服务端出站侧，入站只收字符串+上限
		var sv string
		if err := json.Unmarshal(raw, &sv); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 须为 Markdown 字符串", f.FieldKey))
		}
		if len(sv) > RichtextMaxBytes {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 正文超过 %d KiB 上限", f.FieldKey, RichtextMaxBytes>>10))
		}
	case "number", "rating": // B1-6 批1：rating 与 number 同数据模型
		var nv float64
		if err := json.Unmarshal(raw, &nv); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 须为数字", f.FieldKey))
		}
	case "date":
		var sv string
		if err := json.Unmarshal(raw, &sv); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 须为日期字符串", f.FieldKey))
		}
		if _, err := time.Parse("2006-01-02", sv); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 日期格式须为 YYYY-MM-DD", f.FieldKey))
		}
	case "select", "radio": // B1-6 批1：radio 与 select 同数据模型（单值字符串+选项成员）
		var sv string
		if err := json.Unmarshal(raw, &sv); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 须为字符串选项", f.FieldKey))
		}
		values, _ := optionValues(f.FieldOptions)
		if len(values) > 0 && !contains(values, sv) {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 取值 %q 不在选项内", f.FieldKey, sv))
		}
	case "file": // B1-6 批1：object_key 数组（形态校验；required/MIME 在 validateCustomData）
		var arr []string
		if err := json.Unmarshal(raw, &arr); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 须为 object_key 字符串数组", f.FieldKey))
		}
		for _, k := range arr {
			if k == "" {
				return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 含空 object_key", f.FieldKey))
			}
		}
	case "multi_select":
		var arr []string
		if err := json.Unmarshal(raw, &arr); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 须为字符串数组", f.FieldKey))
		}
		values, _ := optionValues(f.FieldOptions)
		if len(values) > 0 {
			for _, v := range arr {
				if !contains(values, v) {
					return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 取值 %q 不在选项内", f.FieldKey, v))
				}
			}
		}
	case "tips":
		// 说明块无值
	}
	if f.ValidateRegex != "" {
		var sv string
		if err := json.Unmarshal(raw, &sv); err == nil {
			re, err2 := regexp.Compile(f.ValidateRegex)
			if err2 == nil && !re.MatchString(sv) {
				return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 不匹配校验规则", f.FieldKey))
			}
		}
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
