package repo

import (
	"database/sql"

	"learn/smtp-platform/internal/models"
)

type AnalyticsRepo struct {
	db *sql.DB
}

func NewAnalyticsRepo(db *sql.DB) *AnalyticsRepo { return &AnalyticsRepo{db: db} }

func (r *AnalyticsRepo) OverviewTotals() (models.OverviewTotals, error) {
	var out models.OverviewTotals

	if err := r.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&out.UsersTotal); err != nil {
		return out, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE enabled=1`).Scan(&out.UsersActive); err != nil {
		return out, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE created_at >= DATETIME('now', '-24 hours')`).Scan(&out.NewUsers24h); err != nil {
		return out, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM mail_events`).Scan(&out.LogsTotal); err != nil {
		return out, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM mail_events WHERE status='sent' AND created_at >= DATETIME('now', '-24 hours')`).Scan(&out.Sent24h); err != nil {
		return out, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM mail_events WHERE status='failed' AND created_at >= DATETIME('now', '-24 hours')`).Scan(&out.Failed24h); err != nil {
		return out, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM mail_events WHERE status='sent' AND created_at >= DATETIME('now', 'start of month')`).Scan(&out.MonthSent); err != nil {
		return out, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM outbound_queue WHERE status='pending'`).Scan(&out.QueuePending); err != nil {
		return out, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM outbound_queue WHERE status='failed'`).Scan(&out.QueueFailed); err != nil {
		return out, err
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM outbound_queue WHERE status='sent'`).Scan(&out.QueueSent); err != nil {
		return out, err
	}

	return out, nil
}

func (r *AnalyticsRepo) ListUserUsage(limit int) ([]models.UserUsage, error) {
	rows, err := r.db.Query(`SELECT
		u.id,
		u.username,
		COALESCE(u.display_name, ''),
		COALESCE(u.plan_name, 'starter'),
		COALESCE(u.monthly_limit, 10000),
		COALESCE(u.allow_user_smtp, 1),
		COALESCE(SUM(CASE WHEN e.status='sent' AND e.created_at >= DATETIME('now', 'start of month') THEN 1 ELSE 0 END), 0) AS month_sent,
		COALESCE(SUM(CASE WHEN e.status='failed' AND e.created_at >= DATETIME('now', 'start of month') THEN 1 ELSE 0 END), 0) AS month_failed,
		COALESCE(SUM(CASE WHEN e.status='sent' AND e.created_at >= DATETIME('now', '-24 hours') THEN 1 ELSE 0 END), 0) AS day_sent,
		COALESCE(SUM(CASE WHEN e.status='failed' AND e.created_at >= DATETIME('now', '-24 hours') THEN 1 ELSE 0 END), 0) AS day_failed
		FROM users u
		LEFT JOIN mail_events e ON e.user_id = u.id
		GROUP BY u.id, u.username, u.display_name, u.plan_name, u.monthly_limit, u.allow_user_smtp
		ORDER BY month_sent DESC, u.id ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.UserUsage
	for rows.Next() {
		var u models.UserUsage
		var allowUserSMTP int
		if err := rows.Scan(&u.UserID, &u.Username, &u.DisplayName, &u.PlanName, &u.MonthlyLimit, &allowUserSMTP, &u.MonthSent, &u.MonthFailed, &u.DaySent, &u.DayFailed); err != nil {
			return nil, err
		}
		u.AllowUserSMTP = allowUserSMTP == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *AnalyticsRepo) UserUsage(userID int64) (models.UserUsage, error) {
	row := r.db.QueryRow(`SELECT
		u.id,
		u.username,
		COALESCE(u.display_name, ''),
		COALESCE(u.plan_name, 'starter'),
		COALESCE(u.monthly_limit, 10000),
		COALESCE(u.allow_user_smtp, 1),
		COALESCE(SUM(CASE WHEN e.status='sent' AND e.created_at >= DATETIME('now', 'start of month') THEN 1 ELSE 0 END), 0) AS month_sent,
		COALESCE(SUM(CASE WHEN e.status='failed' AND e.created_at >= DATETIME('now', 'start of month') THEN 1 ELSE 0 END), 0) AS month_failed,
		COALESCE(SUM(CASE WHEN e.status='sent' AND e.created_at >= DATETIME('now', '-24 hours') THEN 1 ELSE 0 END), 0) AS day_sent,
		COALESCE(SUM(CASE WHEN e.status='failed' AND e.created_at >= DATETIME('now', '-24 hours') THEN 1 ELSE 0 END), 0) AS day_failed
		FROM users u
		LEFT JOIN mail_events e ON e.user_id = u.id
		WHERE u.id = ?
		GROUP BY u.id, u.username, u.display_name, u.plan_name, u.monthly_limit, u.allow_user_smtp`, userID)

	var u models.UserUsage
	var allowUserSMTP int
	err := row.Scan(&u.UserID, &u.Username, &u.DisplayName, &u.PlanName, &u.MonthlyLimit, &allowUserSMTP, &u.MonthSent, &u.MonthFailed, &u.DaySent, &u.DayFailed)
	u.AllowUserSMTP = allowUserSMTP == 1
	return u, err
}
