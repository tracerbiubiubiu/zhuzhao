package model

import "time"

// P4-3 字典/业务枚举（边界：只做业务枚举/运维参数，不碰权限策略面）。
// 类型/项两级+启停+按 type 拉取（gva 形态）；version 乐观锁。

type DictType struct {
	ID        int64     `json:"id,string"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Remark    string    `json:"remark"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DictItem struct {
	ID        int64     `json:"id,string"`
	TypeCode  string    `json:"type_code"`
	Code      string    `json:"code"`
	Label     string    `json:"label"`
	SortOrder int       `json:"sort_order"`
	Enabled   bool      `json:"enabled"`
	Remark    string    `json:"remark"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DictTypeListResponse / DictItemListResponse 分页（PageData 形态）
type DictTypeListResponse struct {
	List     []*DictType `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

type DictItemListResponse struct {
	List     []*DictItem `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}
