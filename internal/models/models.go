package models

import "time"

type User struct {
	ID            int64     `json:"id"`
	Username      string    `json:"username"`
	PasswordHash  string    `json:"-"`
	DisplayName   string    `json:"display_name"`
	PlanName      string    `json:"plan_name"`
	MonthlyLimit  int       `json:"monthly_limit"`
	RotationOn    bool      `json:"rotation_on"`
	Enabled       bool      `json:"enabled"`
	AllowUserSMTP bool      `json:"allow_user_smtp"`
	LimitPerSec   int       `json:"limit_per_sec"`
	LimitPerMin   int       `json:"limit_per_min"`
	LimitPerHour  int       `json:"limit_per_hour"`
	LimitPerDay   int       `json:"limit_per_day"`
	ThrottleMS    int       `json:"throttle_ms"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type DomainThrottle struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Domain       string    `json:"domain"`
	LimitPerHour int       `json:"limit_per_hour"`
	ThrottleMS   int       `json:"throttle_ms"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type MailEvent struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	MailFrom  string    `json:"mail_from"`
	RcptTo    string    `json:"rcpt_to"`
	Domain    string    `json:"domain"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type QueueItem struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	MailFrom      string    `json:"mail_from"`
	RcptTo        string    `json:"rcpt_to"`
	Data          []byte    `json:"data,omitempty"`
	Status        string    `json:"status"`
	Attempts      int       `json:"attempts"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	LastError     string    `json:"last_error"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PackagePlan struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	MonthlyLimit int       `json:"monthly_limit"`
	LimitPerSec  int       `json:"limit_per_sec"`
	LimitPerMin  int       `json:"limit_per_min"`
	LimitPerHour int       `json:"limit_per_hour"`
	LimitPerDay  int       `json:"limit_per_day"`
	ThrottleMS   int       `json:"throttle_ms"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
}

type UpstreamSMTP struct {
	ID          int64     `json:"id"`
	OwnerUserID int64     `json:"owner_user_id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	Password    string    `json:"password"`
	FromEmail   string    `json:"from_email"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

type UserSMTPAssignment struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	SMTPID    int64     `json:"smtp_id"`
	Weight    int       `json:"weight"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

type OverviewTotals struct {
	UsersTotal   int `json:"users_total"`
	UsersActive  int `json:"users_active"`
	NewUsers24h  int `json:"new_users_24h"`
	LogsTotal    int `json:"logs_total"`
	Sent24h      int `json:"sent_24h"`
	Failed24h    int `json:"failed_24h"`
	MonthSent    int `json:"month_sent"`
	QueuePending int `json:"queue_pending"`
	QueueFailed  int `json:"queue_failed"`
	QueueSent    int `json:"queue_sent"`
}

type UserUsage struct {
	UserID        int64  `json:"user_id"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	PlanName      string `json:"plan_name"`
	MonthlyLimit  int    `json:"monthly_limit"`
	AllowUserSMTP bool   `json:"allow_user_smtp"`
	MonthSent     int    `json:"month_sent"`
	MonthFailed   int    `json:"month_failed"`
	DaySent       int    `json:"day_sent"`
	DayFailed     int    `json:"day_failed"`
}

type AdminOverview struct {
	Totals       OverviewTotals `json:"totals"`
	TopUsers     []UserUsage    `json:"top_users"`
	RecentEvents []MailEvent    `json:"recent_events"`
}
