// S10 B10-2 用户自定义标签（10 号 §3.2 + 附录 A.3 ②）：挂/摘/列表 + 列表按标签筛选。
//
// 拍板点 6 预确认口径：默认共享（无权限模型）+ created_by 留痕；可见性随工单 L2
// （列表筛选走 GetFilter 管线——标签不旁路 L2）。
// 授权口径（与 cc/评论对称）：挂标签/自摘=读可见（协作动作，透明读可见即可协作）；
// 摘他人挂的=update 权（轻治理底线——跨用户标签治理场景出现再升级）。
package ticket

import (
	"context"
	"strconv"
	"strings"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// tagEntityType 工单域标签挂载类型（tags.entity_type——phase6 risk 域复用同表另取值）
const tagEntityType = "ticket"

// AddTags 挂标签（幂等；trim+去重+空滤后入库，单次 ≤20 个/单个 ≤50 字符）。
// 返回挂载后的全量标签行（前端即用）。
func (s *Service) AddTags(ctx context.Context, req *model.AddTicketTagsRequest, actorUserID int64) ([]model.TagRow, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "read", strconv.FormatInt(req.TicketID, 10)); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(req.Tags))
	clean := make([]string, 0, len(req.Tags))
	for _, t := range req.Tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		clean = append(clean, t)
	}
	if len(clean) == 0 {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "tags 清洗后为空（全空白/全重复）")
	}
	if _, err := s.tagRepo.AddTags(ctx, tagEntityType, req.TicketID, actorUserID, clean); err != nil {
		return nil, err
	}
	return s.tagRepo.ListByEntity(ctx, tagEntityType, req.TicketID)
}

// RemoveTag 摘标签（幂等——不存在不算错；摘他人挂的需 update 权）。
func (s *Service) RemoveTag(ctx context.Context, req *model.RemoveTicketTagRequest, actorUserID int64) error {
	tid := strconv.FormatInt(req.TicketID, 10)
	if err := s.authorizeCheck(ctx, actorUserID, "read", tid); err != nil {
		return err
	}
	rows, err := s.tagRepo.ListByEntity(ctx, tagEntityType, req.TicketID)
	if err != nil {
		return err
	}
	var mine *model.TagRow
	for i := range rows {
		if rows[i].Tag == strings.TrimSpace(req.Tag) {
			mine = &rows[i]
			break
		}
	}
	if mine == nil {
		return nil // 幂等：标签本就不在
	}
	if mine.CreatedBy != actorUserID {
		if err := s.authorizeCheck(ctx, actorUserID, "update", tid); err != nil {
			return err
		}
	}
	_, err = s.tagRepo.RemoveTag(ctx, tagEntityType, req.TicketID, strings.TrimSpace(req.Tag))
	return err
}

// ListTags 标签列表（读可见面——非可见用户 404 同形）。
func (s *Service) ListTags(ctx context.Context, ticketID, actorUserID int64) ([]model.TagRow, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "read", strconv.FormatInt(ticketID, 10)); err != nil {
		return nil, err
	}
	return s.tagRepo.ListByEntity(ctx, tagEntityType, ticketID)
}
