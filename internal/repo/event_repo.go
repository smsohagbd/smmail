package repo

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"learn/smtp-platform/internal/models"
)

type MailEventRepo struct {
	db *sql.DB
}

func NewMailEventRepo(db *sql.DB) *MailEventRepo { return &MailEventRepo{db: db} }

func (r *MailEventRepo) Create(e models.MailEvent) error {
	_, err := r.db.Exec(`INSERT INTO mail_events (user_id, mail_from, rcpt_to, domain, status, reason)
		VALUES (?, ?, ?, ?, ?, ?)`,
		e.UserID, e.MailFrom, e.RcptTo, e.Domain, e.Status, e.Reason)
	return err
}

func (r *MailEventRepo) List(limit int, userID int64, offset int) ([]models.MailEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	query := `SELECT id, user_id, mail_from, rcpt_to, domain, status, COALESCE(reason, ''), created_at FROM mail_events`
	args := []any{}
	if userID > 0 {
		query += ` WHERE user_id=?`
		args = append(args, userID)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.MailEvent
	for rows.Next() {
		var e models.MailEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.MailFrom, &e.RcptTo, &e.Domain, &e.Status, &e.Reason, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *MailEventRepo) Count(userID int64) (int, error) {
	query := `SELECT COUNT(*) FROM mail_events`
	args := []any{}
	if userID > 0 {
		query += ` WHERE user_id=?`
		args = append(args, userID)
	}
	var total int
	if err := r.db.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (r *MailEventRepo) DeleteAll(userID int64) error {
	query := `DELETE FROM mail_events`
	args := []any{}
	if userID > 0 {
		query += ` WHERE user_id=?`
		args = append(args, userID)
	}
	_, err := r.db.Exec(query, args...)
	return err
}

func (r *MailEventRepo) DeleteSinceDays(userID int64, days int) error {
	if days <= 0 {
		return fmt.Errorf("days must be positive")
	}
	query := `DELETE FROM mail_events WHERE created_at >= DATETIME('now', ?)`
	args := []any{fmt.Sprintf("-%d days", days)}
	if userID > 0 {
		query += ` AND user_id=?`
		args = append(args, userID)
	}
	_, err := r.db.Exec(query, args...)
	return err
}

func (r *MailEventRepo) DeleteDateRange(userID int64, fromDate, toDate string) error {
	query := `DELETE FROM mail_events WHERE created_at >= DATETIME(?) AND created_at <= DATETIME(?)`
	args := []any{fromDate + " 00:00:00", toDate + " 23:59:59"}
	if userID > 0 {
		query += ` AND user_id=?`
		args = append(args, userID)
	}
	_, err := r.db.Exec(query, args...)
	return err
}

func (r *MailEventRepo) CountSentSince(userID int64, domain string, since time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM mail_events WHERE status='sent' AND user_id=? AND created_at >= ?`
	args := []any{userID, since.UTC().Format("2006-01-02 15:04:05")}
	if strings.TrimSpace(domain) != "" {
		query += ` AND domain=?`
		args = append(args, strings.ToLower(strings.TrimSpace(domain)))
	}
	var out int
	if err := r.db.QueryRow(query, args...).Scan(&out); err != nil {
		return 0, err
	}
	return out, nil
}

func (r *MailEventRepo) LastSentAt(userID int64, domain string) (time.Time, bool, error) {
	query := `SELECT created_at FROM mail_events WHERE status='sent' AND user_id=?`
	args := []any{userID}
	if strings.TrimSpace(domain) != "" {
		query += ` AND domain=?`
		args = append(args, strings.ToLower(strings.TrimSpace(domain)))
	}
	query += ` ORDER BY created_at DESC LIMIT 1`

	var t time.Time
	if err := r.db.QueryRow(query, args...).Scan(&t); err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	return t, true, nil
}
