package model

// OrgMemberRequest 组织成员操作
type OrgMemberRequest struct {
	OrgID     int64 `json:"org_id,string" binding:"required"`
	UserID    int64 `json:"user_id,string" binding:"required"`
	IsPrimary bool  `json:"is_primary"`
}
