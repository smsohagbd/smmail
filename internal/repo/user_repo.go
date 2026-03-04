package repo

import (
	"database/sql"
	"errors"

	"learn/smtp-platform/internal/models"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Create(u models.User) (int64, error) {
	res, err := r.db.Exec(`INSERT INTO users
		(username, password_hash, display_name, plan_name, monthly_limit, rotation_on, enabled, allow_user_smtp, limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.PasswordHash, u.DisplayName, u.PlanName, u.MonthlyLimit, boolToInt(u.RotationOn), boolToInt(u.Enabled), boolToInt(u.AllowUserSMTP), u.LimitPerSec, u.LimitPerMin, u.LimitPerHour, u.LimitPerDay, u.ThrottleMS)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *UserRepo) Update(id int64, u models.User) error {
	_, err := r.db.Exec(`UPDATE users SET
		display_name=?, plan_name=?, monthly_limit=?, rotation_on=?, enabled=?, allow_user_smtp=?, limit_per_sec=?, limit_per_min=?, limit_per_hour=?, limit_per_day=?, throttle_ms=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		u.DisplayName, u.PlanName, u.MonthlyLimit, boolToInt(u.RotationOn), boolToInt(u.Enabled), boolToInt(u.AllowUserSMTP), u.LimitPerSec, u.LimitPerMin, u.LimitPerHour, u.LimitPerDay, u.ThrottleMS, id)
	return err
}

func (r *UserRepo) SetPassword(id int64, passwordHash string) error {
	_, err := r.db.Exec(`UPDATE users SET password_hash=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, passwordHash, id)
	return err
}

func (r *UserRepo) GetByUsername(username string) (models.User, error) {
	row := r.db.QueryRow(`SELECT id, username, password_hash, display_name, plan_name, monthly_limit, rotation_on, enabled, COALESCE(allow_user_smtp, 1), limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, created_at, updated_at
		FROM users WHERE username=?`, username)
	return scanUser(row)
}

func (r *UserRepo) GetByID(id int64) (models.User, error) {
	row := r.db.QueryRow(`SELECT id, username, password_hash, display_name, plan_name, monthly_limit, rotation_on, enabled, COALESCE(allow_user_smtp, 1), limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, created_at, updated_at
		FROM users WHERE id=?`, id)
	return scanUser(row)
}

func (r *UserRepo) List() ([]models.User, error) {
	rows, err := r.db.Query(`SELECT id, username, password_hash, display_name, plan_name, monthly_limit, rotation_on, enabled, COALESCE(allow_user_smtp, 1), limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, created_at, updated_at
		FROM users ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.User
	for rows.Next() {
		var u models.User
		var rotationOn int
		var enabled int
		var allowUserSMTP int
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.PlanName, &u.MonthlyLimit, &rotationOn, &enabled, &allowUserSMTP, &u.LimitPerSec, &u.LimitPerMin, &u.LimitPerHour, &u.LimitPerDay, &u.ThrottleMS, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.RotationOn = rotationOn == 1
		u.Enabled = enabled == 1
		u.AllowUserSMTP = allowUserSMTP == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserRepo) Disable(id int64) error {
	_, err := r.db.Exec(`UPDATE users SET enabled=0, updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

func (r *UserRepo) DeleteHard(id int64) error {
	if _, err := r.db.Exec(`DELETE FROM user_smtp_assignments WHERE user_id=?`, id); err != nil {
		return err
	}
	if _, err := r.db.Exec(`DELETE FROM domain_throttles WHERE user_id=?`, id); err != nil {
		return err
	}
	if _, err := r.db.Exec(`DELETE FROM mail_events WHERE user_id=?`, id); err != nil {
		return err
	}
	if _, err := r.db.Exec(`DELETE FROM outbound_queue WHERE user_id=?`, id); err != nil {
		return err
	}
	_, err := r.db.Exec(`DELETE FROM users WHERE id=?`, id)
	return err
}

func (r *UserRepo) ApplyPackage(userID int64, p models.PackagePlan) error {
	_, err := r.db.Exec(`UPDATE users SET
		plan_name=?, monthly_limit=?, limit_per_sec=?, limit_per_min=?, limit_per_hour=?, limit_per_day=?, throttle_ms=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		p.Name, p.MonthlyLimit, p.LimitPerSec, p.LimitPerMin, p.LimitPerHour, p.LimitPerDay, p.ThrottleMS, userID)
	return err
}

func scanUser(row *sql.Row) (models.User, error) {
	var u models.User
	var rotationOn int
	var enabled int
	var allowUserSMTP int
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.PlanName, &u.MonthlyLimit, &rotationOn, &enabled, &allowUserSMTP, &u.LimitPerSec, &u.LimitPerMin, &u.LimitPerHour, &u.LimitPerDay, &u.ThrottleMS, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return u, sql.ErrNoRows
		}
		return u, err
	}
	u.RotationOn = rotationOn == 1
	u.Enabled = enabled == 1
	u.AllowUserSMTP = allowUserSMTP == 1
	return u, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
