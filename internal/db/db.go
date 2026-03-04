package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func OpenAndMigrate(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// SQLite is single-writer by nature. Keep one open connection to reduce lock contention.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA synchronous = NORMAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			display_name TEXT,
			plan_name TEXT NOT NULL DEFAULT 'starter',
			monthly_limit INTEGER NOT NULL DEFAULT 10000,
			rotation_on INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			allow_user_smtp INTEGER NOT NULL DEFAULT 1,
			limit_per_sec INTEGER NOT NULL DEFAULT 5,
			limit_per_min INTEGER NOT NULL DEFAULT 120,
			limit_per_hour INTEGER NOT NULL DEFAULT 3000,
			limit_per_day INTEGER NOT NULL DEFAULT 20000,
			throttle_ms INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS domain_throttles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			domain TEXT NOT NULL,
			limit_per_hour INTEGER NOT NULL DEFAULT 1000,
			throttle_ms INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, domain),
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS mail_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			mail_from TEXT NOT NULL,
			rcpt_to TEXT NOT NULL,
			domain TEXT NOT NULL,
			status TEXT NOT NULL,
			reason TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS outbound_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			mail_from TEXT NOT NULL,
			rcpt_to TEXT NOT NULL,
			data BLOB NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			attempts INTEGER NOT NULL DEFAULT 0,
			next_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_error TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS package_plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			monthly_limit INTEGER NOT NULL DEFAULT 10000,
			limit_per_sec INTEGER NOT NULL DEFAULT 5,
			limit_per_min INTEGER NOT NULL DEFAULT 120,
			limit_per_hour INTEGER NOT NULL DEFAULT 3000,
			limit_per_day INTEGER NOT NULL DEFAULT 20000,
			throttle_ms INTEGER NOT NULL DEFAULT 0,
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS upstream_smtps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_user_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 587,
			username TEXT NOT NULL,
			password TEXT NOT NULL,
			from_email TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS user_smtp_assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			smtp_id INTEGER NOT NULL,
			weight INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, smtp_id),
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(smtp_id) REFERENCES upstream_smtps(id)
		);`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("migration failed: %w", err)
		}
	}

	if err := addColumnIfMissing(db, `ALTER TABLE users ADD COLUMN plan_name TEXT NOT NULL DEFAULT 'starter';`); err != nil {
		return nil, fmt.Errorf("migration alter users.plan_name failed: %w", err)
	}
	if err := addColumnIfMissing(db, `ALTER TABLE users ADD COLUMN monthly_limit INTEGER NOT NULL DEFAULT 10000;`); err != nil {
		return nil, fmt.Errorf("migration alter users.monthly_limit failed: %w", err)
	}
	if err := addColumnIfMissing(db, `ALTER TABLE users ADD COLUMN rotation_on INTEGER NOT NULL DEFAULT 0;`); err != nil {
		return nil, fmt.Errorf("migration alter users.rotation_on failed: %w", err)
	}
	if err := addColumnIfMissing(db, `ALTER TABLE users ADD COLUMN allow_user_smtp INTEGER NOT NULL DEFAULT 1;`); err != nil {
		return nil, fmt.Errorf("migration alter users.allow_user_smtp failed: %w", err)
	}

	_, err = db.Exec(`INSERT OR IGNORE INTO package_plans 
		(name, monthly_limit, limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, is_default)
		VALUES ('starter', 10000, 5, 120, 3000, 20000, 0, 1)`)
	if err != nil {
		return nil, fmt.Errorf("seed package starter failed: %w", err)
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO package_plans 
		(name, monthly_limit, limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, is_default)
		VALUES ('pro', 100000, 20, 1000, 15000, 100000, 0, 0)`)
	if err != nil {
		return nil, fmt.Errorf("seed package pro failed: %w", err)
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO package_plans 
		(name, monthly_limit, limit_per_sec, limit_per_min, limit_per_hour, limit_per_day, throttle_ms, is_default)
		VALUES ('enterprise', 1000000, 60, 4000, 80000, 500000, 0, 0)`)
	if err != nil {
		return nil, fmt.Errorf("seed package enterprise failed: %w", err)
	}

	return db, nil
}

func addColumnIfMissing(db *sql.DB, stmt string) error {
	_, err := db.Exec(stmt)
	if err == nil {
		return nil
	}
	e := strings.ToLower(err.Error())
	if strings.Contains(e, "duplicate column name") {
		return nil
	}
	return err
}
