package repo

import (
	"database/sql"
	"fmt"
	"time"

	"learn/smtp-platform/internal/models"
)

type QueueRepo struct {
	db *sql.DB
}

func NewQueueRepo(db *sql.DB) *QueueRepo { return &QueueRepo{db: db} }

func (r *QueueRepo) Enqueue(item models.QueueItem) error {
	_, err := r.db.Exec(`INSERT INTO outbound_queue (user_id, mail_from, rcpt_to, data, status)
		VALUES (?, ?, ?, ?, 'pending')`, item.UserID, item.MailFrom, item.RcptTo, item.Data)
	return err
}

func (r *QueueRepo) DequeueBatch(n int) ([]models.QueueItem, error) {
	rows, err := r.db.Query(`SELECT id, user_id, mail_from, rcpt_to, data, status, attempts, next_attempt_at, COALESCE(last_error, ''), created_at, updated_at
		FROM outbound_queue
		WHERE status='pending' AND next_attempt_at <= CURRENT_TIMESTAMP
		ORDER BY id ASC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.QueueItem
	for rows.Next() {
		var q models.QueueItem
		if err := rows.Scan(&q.ID, &q.UserID, &q.MailFrom, &q.RcptTo, &q.Data, &q.Status, &q.Attempts, &q.NextAttemptAt, &q.LastError, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (r *QueueRepo) MarkSent(id int64) error {
	_, err := r.db.Exec(`UPDATE outbound_queue SET status='sent', updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

func (r *QueueRepo) MarkRetry(id int64, errMsg string) error {
	_, err := r.db.Exec(`UPDATE outbound_queue SET attempts=attempts+1, last_error=?, next_attempt_at=DATETIME('now', '+30 seconds'), updated_at=CURRENT_TIMESTAMP WHERE id=?`, errMsg, id)
	return err
}

func (r *QueueRepo) Defer(id int64, errMsg string, wait time.Duration) error {
	if wait < time.Second {
		wait = time.Second
	}
	seconds := int(wait.Seconds())
	_, err := r.db.Exec(`UPDATE outbound_queue 
		SET last_error=?, next_attempt_at=DATETIME('now', ?), updated_at=CURRENT_TIMESTAMP 
		WHERE id=?`, errMsg, fmt.Sprintf("+%d seconds", seconds), id)
	return err
}

func (r *QueueRepo) MarkFailed(id int64, errMsg string) error {
	_, err := r.db.Exec(`UPDATE outbound_queue SET status='failed', attempts=attempts+1, last_error=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, errMsg, id)
	return err
}

func (r *QueueRepo) ListPending(limit int, userID int64, offset int) ([]models.QueueItem, error) {
	if limit <= 0 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	query := `SELECT id, user_id, mail_from, rcpt_to, status, attempts, next_attempt_at, COALESCE(last_error, ''), created_at, updated_at
		FROM outbound_queue WHERE status='pending'`
	args := []any{}
	if userID > 0 {
		query += ` AND user_id=?`
		args = append(args, userID)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.QueueItem{}
	for rows.Next() {
		var q models.QueueItem
		if err := rows.Scan(&q.ID, &q.UserID, &q.MailFrom, &q.RcptTo, &q.Status, &q.Attempts, &q.NextAttemptAt, &q.LastError, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (r *QueueRepo) CountPending(userID int64) (int, error) {
	query := `SELECT COUNT(*) FROM outbound_queue WHERE status='pending'`
	args := []any{}
	if userID > 0 {
		query += ` AND user_id=?`
		args = append(args, userID)
	}
	var total int
	if err := r.db.QueryRow(query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (r *QueueRepo) DeletePendingAll(userID int64) error {
	query := `DELETE FROM outbound_queue WHERE status='pending'`
	args := []any{}
	if userID > 0 {
		query += ` AND user_id=?`
		args = append(args, userID)
	}
	_, err := r.db.Exec(query, args...)
	return err
}

func (r *QueueRepo) DeletePendingSinceDays(userID int64, days int) error {
	if days <= 0 {
		return fmt.Errorf("days must be positive")
	}
	query := `DELETE FROM outbound_queue WHERE status='pending' AND created_at >= DATETIME('now', ?)`
	args := []any{fmt.Sprintf("-%d days", days)}
	if userID > 0 {
		query += ` AND user_id=?`
		args = append(args, userID)
	}
	_, err := r.db.Exec(query, args...)
	return err
}

func (r *QueueRepo) DeletePendingDateRange(userID int64, fromDate, toDate string) error {
	query := `DELETE FROM outbound_queue 
		WHERE status='pending' AND created_at >= DATETIME(?) AND created_at <= DATETIME(?)`
	args := []any{fromDate + " 00:00:00", toDate + " 23:59:59"}
	if userID > 0 {
		query += ` AND user_id=?`
		args = append(args, userID)
	}
	_, err := r.db.Exec(query, args...)
	return err
}
