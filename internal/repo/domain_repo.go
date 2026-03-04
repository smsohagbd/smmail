package repo

import (
	"database/sql"

	"learn/smtp-platform/internal/models"
)

type DomainThrottleRepo struct {
	db *sql.DB
}

func NewDomainThrottleRepo(db *sql.DB) *DomainThrottleRepo { return &DomainThrottleRepo{db: db} }

func (r *DomainThrottleRepo) Upsert(userID int64, domain string, limitPerHour, throttleMS int) error {
	_, err := r.db.Exec(`INSERT INTO domain_throttles (user_id, domain, limit_per_hour, throttle_ms)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, domain)
		DO UPDATE SET limit_per_hour=excluded.limit_per_hour, throttle_ms=excluded.throttle_ms, updated_at=CURRENT_TIMESTAMP`,
		userID, domain, limitPerHour, throttleMS)
	return err
}

func (r *DomainThrottleRepo) Get(userID int64, domain string) (models.DomainThrottle, error) {
	row := r.db.QueryRow(`SELECT id, user_id, domain, limit_per_hour, throttle_ms, created_at, updated_at
		FROM domain_throttles WHERE user_id=? AND domain=?`, userID, domain)
	var d models.DomainThrottle
	err := row.Scan(&d.ID, &d.UserID, &d.Domain, &d.LimitPerHour, &d.ThrottleMS, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (r *DomainThrottleRepo) ListByUser(userID int64) ([]models.DomainThrottle, error) {
	rows, err := r.db.Query(`SELECT id, user_id, domain, limit_per_hour, throttle_ms, created_at, updated_at
		FROM domain_throttles WHERE user_id=? ORDER BY domain`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.DomainThrottle
	for rows.Next() {
		var d models.DomainThrottle
		if err := rows.Scan(&d.ID, &d.UserID, &d.Domain, &d.LimitPerHour, &d.ThrottleMS, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}