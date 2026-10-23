// NotificationPrefService 用户通知偏好服务层（B6-3——handler 经 service 访问 db）。
package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationPrefService struct {
	pool *pgxpool.Pool
}

func NewNotificationPrefService(pool *pgxpool.Pool) *NotificationPrefService {
	return &NotificationPrefService{pool: pool}
}

type OptoutEntry struct {
	BizType     string `json:"biz_type"`
	EventAction string `json:"event_action"`
}

func (s *NotificationPrefService) ListOptouts(ctx context.Context, userID int64) ([]OptoutEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT biz_type, event_action FROM notification_optouts WHERE user_id = $1 ORDER BY biz_type, event_action`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OptoutEntry{}
	for rows.Next() {
		var o OptoutEntry
		if err := rows.Scan(&o.BizType, &o.EventAction); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func (s *NotificationPrefService) SetOptout(ctx context.Context, userID int64, bizType, eventAction string, optOut bool) error {
	if optOut {
		_, err := s.pool.Exec(ctx,
			`INSERT INTO notification_optouts (user_id, biz_type, event_action) VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, biz_type, event_action) DO NOTHING`, userID, bizType, eventAction)
		return err
	}
	_, err := s.pool.Exec(ctx,
		`DELETE FROM notification_optouts WHERE user_id = $1 AND biz_type = $2 AND event_action = $3`,
		userID, bizType, eventAction)
	return err
}
