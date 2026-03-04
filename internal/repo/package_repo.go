package repo

import (
	"database/sql"
	"errors"

	"learn/smtp-platform/internal/models"
)

type PackageRepo struct {
	db *sql.DB
}

func NewPackageRepo(db *sql.DB) *PackageRepo { return &PackageRepo{db: db} }

func (r *PackageRepo) List() ([]models.PackagePlan, error) {
	rows, err := r.db.Query(`SELECT id, name, monthly_limit, limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, is_default, created_at
		FROM package_plans ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.PackagePlan
	for rows.Next() {
		var p models.PackagePlan
		var isDefault int
		if err := rows.Scan(&p.ID, &p.Name, &p.MonthlyLimit, &p.LimitPerSec, &p.LimitPerMin, &p.LimitPerHour, &p.LimitPerDay, &p.ThrottleMS, &isDefault, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.IsDefault = isDefault == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PackageRepo) CreateOrUpdate(p models.PackagePlan) error {
	_, err := r.db.Exec(`INSERT INTO package_plans (name, monthly_limit, limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		ON CONFLICT(name) DO UPDATE SET
		monthly_limit=excluded.monthly_limit,
		limit_per_sec=excluded.limit_per_sec,
		limit_per_min=excluded.limit_per_min,
		limit_per_hour=excluded.limit_per_hour,
		limit_per_day=excluded.limit_per_day,
		throttle_ms=excluded.throttle_ms`,
		p.Name, p.MonthlyLimit, p.LimitPerSec, p.LimitPerMin, p.LimitPerHour, p.LimitPerDay, p.ThrottleMS)
	return err
}

func (r *PackageRepo) SetDefault(name string) error {
	if _, err := r.db.Exec(`UPDATE package_plans SET is_default=0`); err != nil {
		return err
	}
	_, err := r.db.Exec(`UPDATE package_plans SET is_default=1 WHERE name=?`, name)
	return err
}

func (r *PackageRepo) GetDefault() (models.PackagePlan, error) {
	row := r.db.QueryRow(`SELECT id, name, monthly_limit, limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, is_default, created_at
		FROM package_plans WHERE is_default=1 LIMIT 1`)
	var p models.PackagePlan
	var isDefault int
	err := row.Scan(&p.ID, &p.Name, &p.MonthlyLimit, &p.LimitPerSec, &p.LimitPerMin, &p.LimitPerHour, &p.LimitPerDay, &p.ThrottleMS, &isDefault, &p.CreatedAt)
	p.IsDefault = isDefault == 1
	return p, err
}

func (r *PackageRepo) GetByName(name string) (models.PackagePlan, error) {
	row := r.db.QueryRow(`SELECT id, name, monthly_limit, limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, is_default, created_at
		FROM package_plans WHERE name=?`, name)
	var p models.PackagePlan
	var isDefault int
	err := row.Scan(&p.ID, &p.Name, &p.MonthlyLimit, &p.LimitPerSec, &p.LimitPerMin, &p.LimitPerHour, &p.LimitPerDay, &p.ThrottleMS, &isDefault, &p.CreatedAt)
	p.IsDefault = isDefault == 1
	return p, err
}

func (r *PackageRepo) Delete(name string) error {
	p, err := r.GetByName(name)
	if err != nil {
		return err
	}
	if p.IsDefault {
		return errors.New("cannot delete default package")
	}
	_, err = r.db.Exec(`DELETE FROM package_plans WHERE name=?`, name)
	return err
}
