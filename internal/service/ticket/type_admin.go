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
	return s.ticketRepo.UpdateTicketType(ctx, code, req.Name, req.Description, req.States, req.Transitions, req.IsActive, req.Version)
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

// ReplaceTicketTypeFields 全量替换字段集（校验：key 唯一/类型枚举/select 选项/正则可编译）。
// P1-3：req.Version 可选乐观锁，以父类型 ticket_types.version 做 CAS；nil = 旧行为。
func (s *Service) ReplaceTicketTypeFields(ctx context.Context, code string, req *model.ReplaceTypeFieldsRequest) error {
	if req.Version != nil && *req.Version <= 0 {
		return errcode.New(errcode.ErrInvalidParams.Code, "version 须为正整数")
	}
	if _, err := s.ticketRepo.GetTicketType(ctx, code); err != nil {
		return err
	}
	fields, err := validateFieldInputs(req.Fields)
	if err != nil {
		return err
	}
	return s.ticketRepo.ReplaceTypeFields(ctx, code, fields, req.Version)
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
		// select/multi_select 选项规整：接受 ["a","b"] 或 [{"label","value"}]；空选项拒绝
		if in.FieldType == "select" || in.FieldType == "multi_select" {
			values, err := optionValues(in.FieldOptions)
			if err != nil || len(values) == 0 {
				return nil, errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("字段 %q 为 select/multi_select，field_options 须为非空选项数组", in.FieldKey))
			}
		}
		if in.FieldType == "number" && len(in.FieldOptions) > 0 {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 为 number，不接受 field_options", in.FieldKey))
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
	return fields, nil
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
func (s *Service) validateCustomData(ctx context.Context, typeCode string, customData json.RawMessage) error {
	// 总量封顶先于 schema 查询（无字段定义的类型同样受限——拍板 P5）
	if len(customData) > CustomDataMaxBytes {
		return errcode.New(errcode.ErrInvalidParams.Code,
			fmt.Sprintf("custom_data 超过 %d KiB 上限", CustomDataMaxBytes>>10))
	}
	fields, err := s.ticketRepo.ListTicketTypeFields(ctx, typeCode)
	if err != nil || len(fields) == 0 {
		return err // 无字段定义（或读取失败）不校验——读取失败随 Create 一起失败
	}
	if len(customData) == 0 {
		customData = json.RawMessage("{}")
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(customData, &data); err != nil {
		return errcode.New(errcode.ErrInvalidParams.Code, "custom_data 须为 JSON 对象")
	}
	for _, f := range fields {
		// F-B-2 外审 P0 修复（2026-10-22）：field_options 为 dict: 引用时展开为
		// 真实字典项值集（optionValues 只见 ["dict:code"] 字面量，真实值全被拒）
		if f.FieldOptions != nil && (f.FieldType == "select" || f.FieldType == "multi_select") {
			f.FieldOptions = s.expandDictOptions(ctx, f.FieldOptions)
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
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("字段 %q（%s）为必填", f.FieldKey, f.FieldLabel))
		}
		// 空值跳过类型校验（formula 字段空值亦然——required 已豁免，值由
		// applyFormulas 算入；P2-6 清理 2026-10-22：原三重连判第三支不可达）
		if isEmpty {
			continue
		}
		if err := validateFieldValue(f, raw); err != nil {
			return err
		}
	}
	return nil
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
	case "number":
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
	case "select":
		var sv string
		if err := json.Unmarshal(raw, &sv); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 须为字符串选项", f.FieldKey))
		}
		values, _ := optionValues(f.FieldOptions)
		if len(values) > 0 && !contains(values, sv) {
			return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("字段 %q 取值 %q 不在选项内", f.FieldKey, sv))
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
