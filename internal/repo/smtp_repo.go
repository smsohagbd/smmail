package repo

import (
	"database/sql"

	"learn/smtp-platform/internal/models"
)

type SMTPRepo struct {
	db *sql.DB
}

func NewSMTPRepo(db *sql.DB) *SMTPRepo { return &SMTPRepo{db: db} }

func (r *SMTPRepo) Create(s models.UpstreamSMTP) (int64, error) {
	res, err := r.db.Exec(`INSERT INTO upstream_smtps (owner_user_id, name, host, port, username, password, from_email, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.OwnerUserID, s.Name, s.Host, s.Port, s.Username, s.Password, s.FromEmail, boolToInt(s.Enabled))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *SMTPRepo) List(ownerUserID int64) ([]models.UpstreamSMTP, error) {
	rows, err := r.db.Query(`SELECT id, owner_user_id, name, host, port, username, password, from_email, enabled, created_at
		FROM upstream_smtps WHERE owner_user_id=? ORDER BY id DESC`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.UpstreamSMTP
	for rows.Next() {
		var s models.UpstreamSMTP
		var enabled int
		if err := rows.Scan(&s.ID, &s.OwnerUserID, &s.Name, &s.Host, &s.Port, &s.Username, &s.Password, &s.FromEmail, &enabled, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Enabled = enabled == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SMTPRepo) ListAvailableForUser(userID int64) ([]models.UpstreamSMTP, error) {
	rows, err := r.db.Query(`SELECT id, owner_user_id, name, host, port, username, password, from_email, enabled, created_at
		FROM upstream_smtps WHERE enabled=1 AND (owner_user_id=0 OR owner_user_id=?) ORDER BY owner_user_id ASC, id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.UpstreamSMTP
	for rows.Next() {
		var s models.UpstreamSMTP
		var enabled int
		if err := rows.Scan(&s.ID, &s.OwnerUserID, &s.Name, &s.Host, &s.Port, &s.Username, &s.Password, &s.FromEmail, &enabled, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Enabled = enabled == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SMTPRepo) GetByID(id int64) (models.UpstreamSMTP, error) {
	row := r.db.QueryRow(`SELECT id, owner_user_id, name, host, port, username, password, from_email, enabled, created_at
		FROM upstream_smtps WHERE id=?`, id)
	var s models.UpstreamSMTP
	var enabled int
	err := row.Scan(&s.ID, &s.OwnerUserID, &s.Name, &s.Host, &s.Port, &s.Username, &s.Password, &s.FromEmail, &enabled, &s.CreatedAt)
	s.Enabled = enabled == 1
	return s, err
}

func (r *SMTPRepo) AssignToUser(userID, smtpID int64, weight int, enabled bool) error {
	if weight <= 0 {
		weight = 1
	}
	_, err := r.db.Exec(`INSERT INTO user_smtp_assignments (user_id, smtp_id, weight, enabled)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, smtp_id) DO UPDATE SET weight=excluded.weight, enabled=excluded.enabled`, userID, smtpID, weight, boolToInt(enabled))
	return err
}

func (r *SMTPRepo) ListAssigned(userID int64) ([]models.UpstreamSMTP, error) {
	rows, err := r.db.Query(`SELECT s.id, s.owner_user_id, s.name, s.host, s.port, s.username, s.password, s.from_email, s.enabled, s.created_at
		FROM user_smtp_assignments a
		JOIN upstream_smtps s ON s.id=a.smtp_id
		WHERE a.user_id=? AND a.enabled=1 AND s.enabled=1
		ORDER BY a.id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.UpstreamSMTP
	for rows.Next() {
		var s models.UpstreamSMTP
		var enabled int
		if err := rows.Scan(&s.ID, &s.OwnerUserID, &s.Name, &s.Host, &s.Port, &s.Username, &s.Password, &s.FromEmail, &enabled, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Enabled = enabled == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SMTPRepo) UnassignFromUser(userID, smtpID int64) error {
	_, err := r.db.Exec(`DELETE FROM user_smtp_assignments WHERE user_id=? AND smtp_id=?`, userID, smtpID)
	return err
}

func (r *SMTPRepo) DeleteSMTP(smtpID int64) error {
	if _, err := r.db.Exec(`DELETE FROM user_smtp_assignments WHERE smtp_id=?`, smtpID); err != nil {
		return err
	}
	_, err := r.db.Exec(`DELETE FROM upstream_smtps WHERE id=?`, smtpID)
	return err
}
