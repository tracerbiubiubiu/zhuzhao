package model

import "time"

type TicketStats struct {
	AsOf  time.Time         `json:"as_of"`
	Total int64             `json:"total"`
	Items []TicketStatsItem `json:"items"`
}

type TicketStatsItem struct {
	Key             string   `json:"key"`
	Count           int64    `json:"count"`
	InProgressCount int64    `json:"in_progress_count"`
	OverdueCount    int64    `json:"overdue_count"`
	OverdueRate     *float64 `json:"overdue_rate"`
}
