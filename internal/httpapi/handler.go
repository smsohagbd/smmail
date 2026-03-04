package httpapi

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"learn/smtp-platform/internal/config"
	"learn/smtp-platform/internal/models"
	"learn/smtp-platform/internal/repo"
	"learn/smtp-platform/internal/service"
)

type userSession struct {
	UserID    int64
	ExpiresAt time.Time
}

type adminSession struct {
	ExpiresAt time.Time
}

type Handler struct {
	cfg       config.Config
	users     *repo.UserRepo
	domains   *repo.DomainThrottleRepo
	events    *repo.MailEventRepo
	analytics *repo.AnalyticsRepo
	packages  *repo.PackageRepo
	smtpRepo  *repo.SMTPRepo
	queue     *repo.QueueRepo
	auth      *service.AuthService

	sessionMu sync.Mutex
	sessions  map[string]userSession
	adminSess map[string]adminSession
}

func NewHandler(cfg config.Config, users *repo.UserRepo, domains *repo.DomainThrottleRepo, events *repo.MailEventRepo, analytics *repo.AnalyticsRepo, packages *repo.PackageRepo, smtpRepo *repo.SMTPRepo, queue *repo.QueueRepo, auth *service.AuthService) *Handler {
	return &Handler{
		cfg:       cfg,
		users:     users,
		domains:   domains,
		events:    events,
		analytics: analytics,
		packages:  packages,
		smtpRepo:  smtpRepo,
		queue:     queue,
		auth:      auth,
		sessions:  map[string]userSession{},
		adminSess: map[string]adminSession{},
	}
}

func (h *Handler) Router() *gin.Engine {
	r := gin.Default()
	r.Static("/ui", "web/ui")
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/ui/admin.html") })
	r.POST("/api/smtp-test", h.smtpTest)

	r.POST("/api/admin/login", h.adminLogin)

	admin := r.Group("/api/admin", h.adminAuth())
	admin.GET("/overview", h.overview)
	admin.GET("/user-usage", h.listUserUsage)
	admin.GET("/users", h.listUsers)
	admin.POST("/users", h.createUser)
	admin.PUT("/users/:id", h.updateUser)
	admin.POST("/users/:id/package", h.assignPackageToUser)
	admin.POST("/users/:id/password", h.setPassword)
	admin.POST("/users/:id/smtp-mode", h.setUserSMTPMode)
	admin.DELETE("/users/:id", h.deleteUser)
	admin.GET("/users/:id/usage", h.userUsage)
	admin.GET("/users/:id/domain-throttles", h.listDomainThrottles)
	admin.PUT("/users/:id/domain-throttles", h.upsertDomainThrottle)
	admin.GET("/events", h.listEvents)
	admin.POST("/events/delete", h.deleteEventsAdmin)
	admin.GET("/pending", h.listPendingAdmin)
	admin.POST("/pending/delete", h.deletePendingAdmin)
	admin.POST("/smtp-test", h.smtpTest)
	admin.GET("/packages", h.listPackages)
	admin.POST("/packages", h.createOrUpdatePackage)
	admin.POST("/packages/default", h.setDefaultPackage)
	admin.DELETE("/packages/:name", h.deletePackage)
	admin.GET("/smtps", h.listAdminSMTPs)
	admin.POST("/smtps", h.createAdminSMTP)
	admin.DELETE("/smtps/:id", h.deleteSMTPAdmin)
	admin.POST("/smtps/:id/test", h.testSMTPAdmin)
	admin.POST("/users/:id/smtp-assign", h.assignSMTPToUser)
	admin.GET("/users/:id/smtp-assign", h.listUserSMTPAssign)
	admin.DELETE("/users/:id/smtp-assign/:smtp_id", h.unassignSMTPAdmin)

	user := r.Group("/api/user")
	user.POST("/register", h.userRegister)
	user.POST("/login", h.userLogin)
	user.GET("/me", h.userAuth(), h.userMe)
	user.GET("/packages", h.userAuth(), h.userListPackages)
	user.POST("/plan", h.userAuth(), h.userSelectPlan)
	user.GET("/usage", h.userAuth(), h.userUsageSelf)
	user.GET("/events", h.userAuth(), h.userEvents)
	user.POST("/events/delete", h.userAuth(), h.deleteEventsUser)
	user.GET("/pending", h.userAuth(), h.userPending)
	user.POST("/pending/delete", h.userAuth(), h.deletePendingUser)
	user.GET("/smtps", h.userAuth(), h.userListSMTPs)
	user.POST("/smtps", h.userAuth(), h.userCreateSMTP)
	user.DELETE("/smtps/:id", h.userAuth(), h.userDeleteSMTP)
	user.POST("/smtps/:id/test", h.userAuth(), h.userTestSMTP)
	user.GET("/smtps/available", h.userAuth(), h.userAvailableSMTP)
	user.POST("/smtps/assign", h.userAuth(), h.userAssignSMTP)
	user.GET("/smtps/assigned", h.userAuth(), h.userAssignedSMTPs)
	user.DELETE("/smtps/assign/:smtp_id", h.userAuth(), h.userUnassignSMTP)
	user.POST("/rotation", h.userAuth(), h.userSetRotation)

	return r
}

func (h *Handler) adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Admin-Session")
		if token == "" {
			authz := c.GetHeader("Authorization")
			if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				token = strings.TrimSpace(authz[7:])
			}
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing admin session"})
			return
		}
		h.sessionMu.Lock()
		sess, ok := h.adminSess[token]
		h.sessionMu.Unlock()
		if !ok || time.Now().After(sess.ExpiresAt) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired admin session"})
			return
		}
		c.Next()
	}
}

func (h *Handler) userAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-User-Token")
		if token == "" {
			authz := c.GetHeader("Authorization")
			if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				token = strings.TrimSpace(authz[7:])
			}
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing user token"})
			return
		}
		h.sessionMu.Lock()
		sess, ok := h.sessions[token]
		h.sessionMu.Unlock()
		if !ok || time.Now().After(sess.ExpiresAt) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired user token"})
			return
		}
		c.Set("user_id", sess.UserID)
		c.Next()
	}
}

func (h *Handler) adminLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Username) != h.cfg.AdminUsername || req.Password != h.cfg.AdminPassword {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	token, err := randomToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}
	exp := time.Now().Add(24 * time.Hour)
	h.sessionMu.Lock()
	h.adminSess[token] = adminSession{ExpiresAt: exp}
	h.sessionMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"token": token, "expires_at": exp.UTC().Format(time.RFC3339)})
}

func (h *Handler) overview(c *gin.Context) {
	totals, err := h.analytics.OverviewTotals()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	users, err := h.analytics.ListUserUsage(10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	events, err := h.events.List(20, 0, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.AdminOverview{Totals: totals, TopUsers: users, RecentEvents: events})
}

func (h *Handler) listUserUsage(c *gin.Context) {
	limit := parseInt(c.Query("limit"), 200)
	items, err := h.analytics.ListUserUsage(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) userUsage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	usage, err := h.analytics.UserUsage(id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, usage)
}

func (h *Handler) listUsers(c *gin.Context) {
	users, err := h.users.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h *Handler) createUser(c *gin.Context) {
	var req struct {
		Username      string `json:"username" binding:"required"`
		Password      string `json:"password" binding:"required"`
		DisplayName   string `json:"display_name"`
		PlanName      string `json:"plan_name"`
		MonthlyLimit  int    `json:"monthly_limit"`
		RotationOn    bool   `json:"rotation_on"`
		Enabled       bool   `json:"enabled"`
		AllowUserSMTP *bool  `json:"allow_user_smtp"`
		LimitPerSec   int    `json:"limit_per_sec"`
		LimitPerMin   int    `json:"limit_per_min"`
		LimitPerHour  int    `json:"limit_per_hour"`
		LimitPerDay   int    `json:"limit_per_day"`
		ThrottleMS    int    `json:"throttle_ms"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hash, err := service.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}

	plan := defaultIfEmpty(req.PlanName, "starter")
	monthly := defaultIfZero(req.MonthlyLimit, 10000)
	if req.PlanName == "" {
		if p, err := h.packages.GetDefault(); err == nil {
			plan = p.Name
			monthly = p.MonthlyLimit
			req.LimitPerSec = p.LimitPerSec
			req.LimitPerMin = p.LimitPerMin
			req.LimitPerHour = p.LimitPerHour
			req.LimitPerDay = p.LimitPerDay
			req.ThrottleMS = p.ThrottleMS
		}
	}

	u := models.User{
		Username:      req.Username,
		PasswordHash:  hash,
		DisplayName:   req.DisplayName,
		PlanName:      plan,
		MonthlyLimit:  monthly,
		RotationOn:    req.RotationOn,
		Enabled:       req.Enabled,
		AllowUserSMTP: true,
		LimitPerSec:   defaultIfZero(req.LimitPerSec, 5),
		LimitPerMin:   defaultIfZero(req.LimitPerMin, 120),
		LimitPerHour:  defaultIfZero(req.LimitPerHour, 3000),
		LimitPerDay:   defaultIfZero(req.LimitPerDay, 20000),
		ThrottleMS:    req.ThrottleMS,
	}
	if req.AllowUserSMTP != nil {
		u.AllowUserSMTP = *req.AllowUserSMTP
	}
	id, err := h.users.Create(u)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) updateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		DisplayName   string `json:"display_name"`
		PlanName      string `json:"plan_name"`
		MonthlyLimit  int    `json:"monthly_limit"`
		RotationOn    bool   `json:"rotation_on"`
		Enabled       bool   `json:"enabled"`
		AllowUserSMTP *bool  `json:"allow_user_smtp"`
		LimitPerSec   int    `json:"limit_per_sec"`
		LimitPerMin   int    `json:"limit_per_min"`
		LimitPerHour  int    `json:"limit_per_hour"`
		LimitPerDay   int    `json:"limit_per_day"`
		ThrottleMS    int    `json:"throttle_ms"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, err := h.users.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u.DisplayName = req.DisplayName
	u.PlanName = defaultIfEmpty(req.PlanName, "starter")
	u.MonthlyLimit = defaultIfZero(req.MonthlyLimit, 10000)
	u.RotationOn = req.RotationOn
	u.Enabled = req.Enabled
	if req.AllowUserSMTP != nil {
		u.AllowUserSMTP = *req.AllowUserSMTP
	}
	u.LimitPerSec = defaultIfZero(req.LimitPerSec, 5)
	u.LimitPerMin = defaultIfZero(req.LimitPerMin, 120)
	u.LimitPerHour = defaultIfZero(req.LimitPerHour, 3000)
	u.LimitPerDay = defaultIfZero(req.LimitPerDay, 20000)
	u.ThrottleMS = req.ThrottleMS
	err = h.users.Update(id, u)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) setUserSMTPMode(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		AllowUserSMTP bool `json:"allow_user_smtp"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, err := h.users.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u.AllowUserSMTP = req.AllowUserSMTP
	if !req.AllowUserSMTP {
		u.RotationOn = false
	}
	if err := h.users.Update(id, u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) assignPackageToUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	var req struct {
		PackageName string `json:"package_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pkg, err := h.packages.GetByName(strings.TrimSpace(req.PackageName))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "package not found"})
		return
	}
	if err := h.users.ApplyPackage(userID, pkg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) setPassword(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hash, err := service.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}
	if err := h.users.SetPassword(id, hash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) deleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.users.DeleteHard(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) listDomainThrottles(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	items, err := h.domains.ListByUser(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) upsertDomainThrottle(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	var req struct {
		Domain       string `json:"domain" binding:"required"`
		LimitPerHour int    `json:"limit_per_hour"`
		ThrottleMS   int    `json:"throttle_ms"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.domains.Upsert(id, req.Domain, defaultIfZero(req.LimitPerHour, 1000), req.ThrottleMS); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) listEvents(c *gin.Context) {
	limit := parseInt(c.Query("limit"), 25)
	userID := int64(parseInt(c.Query("user_id"), 0))
	offset := parseInt(c.Query("offset"), 0)
	items, err := h.events.List(limit, userID, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	total, err := h.events.Count(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) deleteEventsAdmin(c *gin.Context) {
	var req struct {
		Mode     string `json:"mode" binding:"required"`
		UserID   int64  `json:"user_id"`
		FromDate string `json:"from_date"`
		ToDate   string `json:"to_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var err error
	switch req.Mode {
	case "all":
		err = h.events.DeleteAll(req.UserID)
	case "last_7_days":
		err = h.events.DeleteSinceDays(req.UserID, 7)
	case "last_15_days":
		err = h.events.DeleteSinceDays(req.UserID, 15)
	case "custom":
		if strings.TrimSpace(req.FromDate) == "" || strings.TrimSpace(req.ToDate) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from_date and to_date are required for custom mode"})
			return
		}
		err = h.events.DeleteDateRange(req.UserID, req.FromDate, req.ToDate)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) smtpTest(c *gin.Context) {
	var req struct {
		Host     string `json:"host" binding:"required"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		From     string `json:"from" binding:"required"`
		To       string `json:"to" binding:"required"`
		Subject  string `json:"subject"`
		Body     string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	port := req.Port
	if port == 0 {
		port = 25
	}
	recipients := splitRecipients(req.To)
	if len(recipients) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one recipient is required"})
		return
	}
	addr := fmt.Sprintf("%s:%d", req.Host, port)
	msg := []byte("From: " + req.From + "\r\n" +
		"To: " + strings.Join(recipients, ",") + "\r\n" +
		"Subject: " + req.Subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		req.Body + "\r\n")
	var auth smtp.Auth
	if req.Username != "" {
		auth = smtp.PlainAuth("", req.Username, req.Password, req.Host)
	}
	if err := smtp.SendMail(addr, auth, req.From, recipients, msg); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "test email sent"})
}

func (h *Handler) listPackages(c *gin.Context) {
	items, err := h.packages.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) createOrUpdatePackage(c *gin.Context) {
	var req struct {
		Name         string `json:"name" binding:"required"`
		MonthlyLimit int    `json:"monthly_limit"`
		LimitPerSec  int    `json:"limit_per_sec"`
		LimitPerMin  int    `json:"limit_per_min"`
		LimitPerHour int    `json:"limit_per_hour"`
		LimitPerDay  int    `json:"limit_per_day"`
		ThrottleMS   int    `json:"throttle_ms"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.packages.CreateOrUpdate(models.PackagePlan{
		Name:         req.Name,
		MonthlyLimit: defaultIfZero(req.MonthlyLimit, 10000),
		LimitPerSec:  defaultIfZero(req.LimitPerSec, 5),
		LimitPerMin:  defaultIfZero(req.LimitPerMin, 120),
		LimitPerHour: defaultIfZero(req.LimitPerHour, 3000),
		LimitPerDay:  defaultIfZero(req.LimitPerDay, 20000),
		ThrottleMS:   req.ThrottleMS,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) setDefaultPackage(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.packages.SetDefault(req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) deletePackage(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid package name"})
		return
	}
	if err := h.packages.Delete(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) listAdminSMTPs(c *gin.Context) {
	items, err := h.smtpRepo.List(0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) createAdminSMTP(c *gin.Context) {
	h.createSMTP(c, 0)
}

func (h *Handler) deleteSMTPAdmin(c *gin.Context) {
	smtpID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid smtp id"})
		return
	}
	if err := h.smtpRepo.DeleteSMTP(smtpID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) testSMTPAdmin(c *gin.Context) {
	smtpID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid smtp id"})
		return
	}
	acc, err := h.smtpRepo.GetByID(smtpID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "smtp not found"})
		return
	}
	var req struct {
		To      string `json:"to" binding:"required"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Subject) == "" {
		req.Subject = "SMTP test"
	}
	if strings.TrimSpace(req.Body) == "" {
		req.Body = "Test email from admin SMTP panel."
	}
	if err := sendStoredSMTPTest(acc, req.To, req.Subject, req.Body); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "test email sent"})
}

func (h *Handler) assignSMTPToUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	var req struct {
		SMTPID  int64 `json:"smtp_id"`
		Weight  int   `json:"weight"`
		Enabled bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := h.users.GetByID(userID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	smtpItem, err := h.smtpRepo.GetByID(req.SMTPID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "smtp not found"})
		return
	}
	if smtpItem.OwnerUserID != 0 && smtpItem.OwnerUserID != userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "smtp account is owned by another user"})
		return
	}
	if err := h.smtpRepo.AssignToUser(userID, req.SMTPID, defaultIfZero(req.Weight, 1), req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) listUserSMTPAssign(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	items, err := h.smtpRepo.ListAssigned(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) unassignSMTPAdmin(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	smtpID, err := strconv.ParseInt(c.Param("smtp_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid smtp id"})
		return
	}
	if err := h.smtpRepo.UnassignFromUser(userID, smtpID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) userRegister(c *gin.Context) {
	var req struct {
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hash, err := service.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}
	pkg, err := h.packages.GetDefault()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "default package not configured"})
		return
	}

	u := models.User{
		Username:      req.Username,
		PasswordHash:  hash,
		DisplayName:   req.DisplayName,
		PlanName:      pkg.Name,
		MonthlyLimit:  pkg.MonthlyLimit,
		RotationOn:    false,
		Enabled:       true,
		AllowUserSMTP: true,
		LimitPerSec:   pkg.LimitPerSec,
		LimitPerMin:   pkg.LimitPerMin,
		LimitPerHour:  pkg.LimitPerHour,
		LimitPerDay:   pkg.LimitPerDay,
		ThrottleMS:    pkg.ThrottleMS,
	}
	id, err := h.users.Create(u)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "package": pkg.Name})
}

func (h *Handler) userLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, err := h.auth.Validate(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	token, err := randomToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}
	exp := time.Now().Add(24 * time.Hour)
	h.sessionMu.Lock()
	h.sessions[token] = userSession{UserID: u.ID, ExpiresAt: exp}
	h.sessionMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"token": token, "expires_at": exp.UTC().Format(time.RFC3339), "user_id": u.ID})
}

func (h *Handler) userMe(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.users.GetByID(userID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, u)
}

func (h *Handler) userListPackages(c *gin.Context) {
	items, err := h.packages.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) userSelectPlan(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req struct {
		PackageName string `json:"package_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pkg, err := h.packages.GetByName(strings.TrimSpace(req.PackageName))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "package not found"})
		return
	}
	if err := h.users.ApplyPackage(userID, pkg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) userUsageSelf(c *gin.Context) {
	userID := c.GetInt64("user_id")
	usage, err := h.analytics.UserUsage(userID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, usage)
}

func (h *Handler) userEvents(c *gin.Context) {
	userID := c.GetInt64("user_id")
	limit := parseInt(c.Query("limit"), 25)
	offset := parseInt(c.Query("offset"), 0)
	items, err := h.events.List(limit, userID, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	total, err := h.events.Count(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) deleteEventsUser(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req struct {
		Mode     string `json:"mode" binding:"required"`
		FromDate string `json:"from_date"`
		ToDate   string `json:"to_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var err error
	switch req.Mode {
	case "all":
		err = h.events.DeleteAll(userID)
	case "last_7_days":
		err = h.events.DeleteSinceDays(userID, 7)
	case "last_15_days":
		err = h.events.DeleteSinceDays(userID, 15)
	case "custom":
		if strings.TrimSpace(req.FromDate) == "" || strings.TrimSpace(req.ToDate) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from_date and to_date are required for custom mode"})
			return
		}
		err = h.events.DeleteDateRange(userID, req.FromDate, req.ToDate)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) listPendingAdmin(c *gin.Context) {
	limit := parseInt(c.Query("limit"), 25)
	userID := int64(parseInt(c.Query("user_id"), 0))
	offset := parseInt(c.Query("offset"), 0)
	items, err := h.queue.ListPending(limit, userID, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	total, err := h.queue.CountPending(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) deletePendingAdmin(c *gin.Context) {
	var req struct {
		Mode     string `json:"mode" binding:"required"`
		UserID   int64  `json:"user_id"`
		FromDate string `json:"from_date"`
		ToDate   string `json:"to_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var err error
	switch req.Mode {
	case "all":
		err = h.queue.DeletePendingAll(req.UserID)
	case "last_7_days":
		err = h.queue.DeletePendingSinceDays(req.UserID, 7)
	case "last_15_days":
		err = h.queue.DeletePendingSinceDays(req.UserID, 15)
	case "custom":
		if strings.TrimSpace(req.FromDate) == "" || strings.TrimSpace(req.ToDate) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from_date and to_date are required for custom mode"})
			return
		}
		err = h.queue.DeletePendingDateRange(req.UserID, req.FromDate, req.ToDate)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) userPending(c *gin.Context) {
	userID := c.GetInt64("user_id")
	limit := parseInt(c.Query("limit"), 25)
	offset := parseInt(c.Query("offset"), 0)
	items, err := h.queue.ListPending(limit, userID, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	total, err := h.queue.CountPending(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (h *Handler) deletePendingUser(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req struct {
		Mode     string `json:"mode" binding:"required"`
		FromDate string `json:"from_date"`
		ToDate   string `json:"to_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var err error
	switch req.Mode {
	case "all":
		err = h.queue.DeletePendingAll(userID)
	case "last_7_days":
		err = h.queue.DeletePendingSinceDays(userID, 7)
	case "last_15_days":
		err = h.queue.DeletePendingSinceDays(userID, 15)
	case "custom":
		if strings.TrimSpace(req.FromDate) == "" || strings.TrimSpace(req.ToDate) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from_date and to_date are required for custom mode"})
			return
		}
		err = h.queue.DeletePendingDateRange(userID, req.FromDate, req.ToDate)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) userListSMTPs(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !u.AllowUserSMTP {
		c.JSON(http.StatusOK, []models.UpstreamSMTP{})
		return
	}
	items, err := h.smtpRepo.List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) userAvailableSMTP(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items, err := h.smtpRepo.ListAvailableForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !u.AllowUserSMTP {
		filtered := make([]models.UpstreamSMTP, 0, len(items))
		for _, it := range items {
			if it.OwnerUserID == 0 {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) userAssignedSMTPs(c *gin.Context) {
	userID := c.GetInt64("user_id")
	items, err := h.smtpRepo.ListAssigned(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) userCreateSMTP(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !u.AllowUserSMTP {
		c.JSON(http.StatusForbidden, gin.H{"error": "your account is restricted to system SMTP only"})
		return
	}
	h.createSMTP(c, userID)
}

func (h *Handler) userDeleteSMTP(c *gin.Context) {
	userID := c.GetInt64("user_id")
	smtpID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid smtp id"})
		return
	}
	smtpItem, err := h.smtpRepo.GetByID(smtpID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "smtp not found"})
		return
	}
	if smtpItem.OwnerUserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not allowed"})
		return
	}
	if err := h.smtpRepo.DeleteSMTP(smtpID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) userTestSMTP(c *gin.Context) {
	userID := c.GetInt64("user_id")
	smtpID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid smtp id"})
		return
	}
	acc, err := h.smtpRepo.GetByID(smtpID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "smtp not found"})
		return
	}
	if acc.OwnerUserID != userID {
		assigned, err := h.smtpRepo.ListAssigned(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		allowed := false
		for _, s := range assigned {
			if s.ID == smtpID {
				allowed = true
				break
			}
		}
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "not allowed"})
			return
		}
	}
	var req struct {
		To      string `json:"to" binding:"required"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Subject) == "" {
		req.Subject = "SMTP test"
	}
	if strings.TrimSpace(req.Body) == "" {
		req.Body = "Test email from user SMTP panel."
	}
	if err := sendStoredSMTPTest(acc, req.To, req.Subject, req.Body); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "test email sent"})
}

func (h *Handler) userAssignSMTP(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var req struct {
		SMTPID   int64 `json:"smtp_id"`
		Weight   int   `json:"weight"`
		Rotation bool  `json:"rotation"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	smtpItem, err := h.smtpRepo.GetByID(req.SMTPID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "smtp not found"})
		return
	}
	if !u.AllowUserSMTP && smtpItem.OwnerUserID != 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "only system SMTP can be assigned for this account"})
		return
	}
	if smtpItem.OwnerUserID != 0 && smtpItem.OwnerUserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not allowed to assign this smtp"})
		return
	}
	if err := h.smtpRepo.AssignToUser(userID, req.SMTPID, defaultIfZero(req.Weight, 1), true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.Rotation {
		u, err := h.users.GetByID(userID)
		if err == nil {
			u.RotationOn = true
			_ = h.users.Update(userID, u)
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) userSetRotation(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !u.AllowUserSMTP && req.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "rotation is disabled for system SMTP only accounts"})
		return
	}
	u.RotationOn = req.Enabled
	if err := h.users.Update(userID, u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) userUnassignSMTP(c *gin.Context) {
	userID := c.GetInt64("user_id")
	smtpID, err := strconv.ParseInt(c.Param("smtp_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid smtp id"})
		return
	}
	if err := h.smtpRepo.UnassignFromUser(userID, smtpID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) createSMTP(c *gin.Context, ownerUserID int64) {
	var req struct {
		Name      string `json:"name" binding:"required"`
		Host      string `json:"host" binding:"required"`
		Port      int    `json:"port"`
		Username  string `json:"username" binding:"required"`
		Password  string `json:"password" binding:"required"`
		FromEmail string `json:"from_email" binding:"required"`
		Enabled   bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	check := models.UpstreamSMTP{
		OwnerUserID: ownerUserID,
		Name:        req.Name,
		Host:        req.Host,
		Port:        defaultIfZero(req.Port, 587),
		Username:    req.Username,
		Password:    req.Password,
		FromEmail:   req.FromEmail,
		Enabled:     req.Enabled,
	}
	if err := validateSMTPCredentials(check); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "smtp validation failed: " + err.Error()})
		return
	}
	id, err := h.smtpRepo.Create(models.UpstreamSMTP{
		OwnerUserID: ownerUserID,
		Name:        req.Name,
		Host:        req.Host,
		Port:        defaultIfZero(req.Port, 587),
		Username:    req.Username,
		Password:    req.Password,
		FromEmail:   req.FromEmail,
		Enabled:     req.Enabled,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if ownerUserID > 0 {
		_ = h.smtpRepo.AssignToUser(ownerUserID, id, 1, true)
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func validateSMTPCredentials(acc models.UpstreamSMTP) error {
	addr := fmt.Sprintf("%s:%d", strings.TrimSpace(acc.Host), acc.Port)
	conn, err := net.DialTimeout("tcp", addr, 8*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, strings.TrimSpace(acc.Host))
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: strings.TrimSpace(acc.Host)}); err != nil {
			return err
		}
	}
	if ok, _ := client.Extension("AUTH"); ok && strings.TrimSpace(acc.Username) != "" {
		auth := smtp.PlainAuth("", acc.Username, acc.Password, strings.TrimSpace(acc.Host))
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	from := strings.TrimSpace(acc.FromEmail)
	if from == "" {
		return fmt.Errorf("from email is required")
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Reset(); err != nil {
		return err
	}
	return client.Quit()
}

func sendStoredSMTPTest(acc models.UpstreamSMTP, to, subject, body string) error {
	recipients := splitRecipients(to)
	if len(recipients) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	from := strings.TrimSpace(acc.FromEmail)
	if from == "" {
		return fmt.Errorf("smtp from email is empty")
	}
	addr := fmt.Sprintf("%s:%d", strings.TrimSpace(acc.Host), acc.Port)
	msg := []byte("From: <" + from + ">\r\n" +
		"To: " + strings.Join(recipients, ",") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body + "\r\n")
	var auth smtp.Auth
	if strings.TrimSpace(acc.Username) != "" {
		auth = smtp.PlainAuth("", acc.Username, acc.Password, strings.TrimSpace(acc.Host))
	}
	return smtp.SendMail(addr, auth, from, recipients, msg)
}

func splitRecipients(raw string) []string {
	raw = strings.ReplaceAll(raw, "\n", ",")
	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseInt(v string, fallback int) int {
	x, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return x
}

func defaultIfZero(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	return v
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
