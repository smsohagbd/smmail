package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"learn/smtp-platform/internal/models"
	"learn/smtp-platform/internal/repo"
)

type DeliveryService struct {
	queue    *repo.QueueRepo
	event    *repo.MailEventRepo
	users    *repo.UserRepo
	domains  *repo.DomainThrottleRepo
	smtpRepo *repo.SMTPRepo

	mu      sync.Mutex
	rrIndex map[int64]int
}

func NewDeliveryService(queue *repo.QueueRepo, event *repo.MailEventRepo, users *repo.UserRepo, domains *repo.DomainThrottleRepo, smtpRepo *repo.SMTPRepo) *DeliveryService {
	return &DeliveryService{queue: queue, event: event, users: users, domains: domains, smtpRepo: smtpRepo, rrIndex: map[int64]int{}}
}

func (s *DeliveryService) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.process()
		}
	}
}

func (s *DeliveryService) process() {
	items, err := s.queue.DequeueBatch(30)
	if err != nil {
		log.Printf("delivery dequeue error: %v", err)
		return
	}
	for _, item := range items {
		held, err := s.holdIfLimited(item)
		if err != nil {
			log.Printf("delivery hold check failed queue_id=%d err=%v", item.ID, err)
			_ = s.queue.MarkRetry(item.ID, err.Error())
			continue
		}
		if held {
			continue
		}

		if err := s.deliver(item); err != nil {
			log.Printf("delivery failed queue_id=%d rcpt=%s attempts=%d err=%v", item.ID, item.RcptTo, item.Attempts+1, err)
			if item.Attempts >= 4 {
				_ = s.queue.MarkFailed(item.ID, err.Error())
				_ = s.event.Create(models.MailEvent{UserID: item.UserID, MailFrom: item.MailFrom, RcptTo: item.RcptTo, Domain: extractDomain(item.RcptTo), Status: "failed", Reason: err.Error()})
			} else {
				_ = s.queue.MarkRetry(item.ID, err.Error())
			}
			continue
		}

		log.Printf("delivery sent queue_id=%d rcpt=%s", item.ID, item.RcptTo)
		_ = s.queue.MarkSent(item.ID)
		_ = s.event.Create(models.MailEvent{UserID: item.UserID, MailFrom: item.MailFrom, RcptTo: item.RcptTo, Domain: extractDomain(item.RcptTo), Status: "sent", Reason: ""})
	}
}

func (s *DeliveryService) holdIfLimited(item models.QueueItem) (bool, error) {
	user, err := s.users.GetByID(item.UserID)
	if err != nil {
		return false, err
	}

	domain := extractDomain(item.RcptTo)
	domainPerHour := 0
	domainThrottleMS := 0
	if domain != "" {
		if rule, err := s.domains.Get(item.UserID, domain); err == nil {
			domainPerHour = rule.LimitPerHour
			domainThrottleMS = rule.ThrottleMS
		} else if err != sql.ErrNoRows {
			return false, err
		}
	}

	now := time.Now()
	if user.ThrottleMS > 0 {
		last, ok, err := s.event.LastSentAt(item.UserID, "")
		if err != nil {
			return false, err
		}
		if ok {
			wait := time.Duration(user.ThrottleMS)*time.Millisecond - now.Sub(last)
			if wait > 0 {
				return true, s.deferWithReason(item, fmt.Sprintf("user throttle active (%dms)", user.ThrottleMS), wait)
			}
		}
	}

	if domain != "" && domainThrottleMS > 0 {
		last, ok, err := s.event.LastSentAt(item.UserID, domain)
		if err != nil {
			return false, err
		}
		if ok {
			wait := time.Duration(domainThrottleMS)*time.Millisecond - now.Sub(last)
			if wait > 0 {
				return true, s.deferWithReason(item, fmt.Sprintf("domain throttle active (%dms) for %s", domainThrottleMS, domain), wait)
			}
		}
	}

	type limitCheck struct {
		limit  int
		window time.Duration
		domain string
		reason string
	}
	checks := []limitCheck{
		{limit: user.LimitPerSec, window: time.Second, reason: "user per-second limit reached"},
		{limit: user.LimitPerMin, window: time.Minute, reason: "user per-minute limit reached"},
		{limit: user.LimitPerHour, window: time.Hour, reason: "user per-hour limit reached"},
		{limit: user.LimitPerDay, window: 24 * time.Hour, reason: "user per-day limit reached"},
	}
	if domain != "" && domainPerHour > 0 {
		checks = append(checks, limitCheck{
			limit:  domainPerHour,
			window: time.Hour,
			domain: domain,
			reason: fmt.Sprintf("domain per-hour limit reached for %s", domain),
		})
	}

	for _, c := range checks {
		if c.limit <= 0 {
			continue
		}
		count, err := s.event.CountSentSince(item.UserID, c.domain, now.Add(-c.window))
		if err != nil {
			return false, err
		}
		if count >= c.limit {
			return true, s.deferWithReason(item, c.reason, c.window)
		}
	}
	return false, nil
}

func (s *DeliveryService) deferWithReason(item models.QueueItem, reason string, wait time.Duration) error {
	if err := s.queue.Defer(item.ID, reason, wait); err != nil {
		return err
	}
	_ = s.event.Create(models.MailEvent{
		UserID:   item.UserID,
		MailFrom: item.MailFrom,
		RcptTo:   item.RcptTo,
		Domain:   extractDomain(item.RcptTo),
		Status:   "queued",
		Reason:   "held: " + reason,
	})
	return nil
}

func (s *DeliveryService) deliver(item models.QueueItem) error {
	user, err := s.users.GetByID(item.UserID)
	if err != nil {
		return err
	}

	assigned, err := s.smtpRepo.ListAssigned(item.UserID)
	if err != nil {
		return err
	}

	if len(assigned) == 0 {
		return fmt.Errorf("no assigned/enabled upstream smtp for user %d", item.UserID)
	}

	if user.RotationOn {
		idx := s.nextRR(item.UserID, len(assigned))
		if err := s.deliverByAccount(item, assigned[idx]); err == nil {
			return nil
		}
		for i := 0; i < len(assigned); i++ {
			if i == idx {
				continue
			}
			if err := s.deliverByAccount(item, assigned[i]); err == nil {
				return nil
			}
		}
		return fmt.Errorf("all assigned smtp accounts failed")
	}

	// Non-rotation mode: always use first assigned account, fallback to others on failure.
	if err := s.deliverByAccount(item, assigned[0]); err == nil {
		return nil
	}
	for i := 1; i < len(assigned); i++ {
		if err := s.deliverByAccount(item, assigned[i]); err == nil {
			return nil
		}
	}
	return fmt.Errorf("assigned smtp accounts failed")
}

func (s *DeliveryService) deliverByAccount(item models.QueueItem, acc models.UpstreamSMTP) error {
	addr := fmt.Sprintf("%s:%d", acc.Host, acc.Port)
	auth := smtp.PlainAuth("", acc.Username, acc.Password, acc.Host)
	from := item.MailFrom
	if acc.FromEmail != "" {
		from = acc.FromEmail
	}
	msg := rewriteFromHeader(item.Data, from)
	return smtp.SendMail(addr, auth, from, []string{item.RcptTo}, msg)
}

func (s *DeliveryService) nextRR(userID int64, n int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.rrIndex[userID]
	if idx >= n {
		idx = 0
	}
	s.rrIndex[userID] = (idx + 1) % n
	return idx
}

func rewriteFromHeader(data []byte, fromEmail string) []byte {
	if fromEmail == "" {
		return data
	}
	lines := splitLines(string(data))
	if len(lines) == 0 {
		return data
	}

	headerEnd := -1
	for i, ln := range lines {
		if ln == "" {
			headerEnd = i
			break
		}
	}
	if headerEnd < 0 {
		return data
	}

	replaced := false
	for i := 0; i < headerEnd; i++ {
		lower := toLowerPrefix(lines[i], 5)
		if lower == "from:" {
			lines[i] = "From: <" + strings.TrimSpace(fromEmail) + ">"
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines[:0], append([]string{"From: <" + strings.TrimSpace(fromEmail) + ">"}, lines...)...)
	}
	return []byte(joinCRLF(lines))
}

func splitLines(s string) []string {
	out := []string{}
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' {
			continue
		}
		if s[i] == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(s[i])
	}
	out = append(out, cur)
	return out
}

func joinCRLF(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := ""
	for i, ln := range lines {
		out += ln
		if i < len(lines)-1 {
			out += "\r\n"
		}
	}
	return out
}

func toLowerPrefix(s string, n int) string {
	if len(s) < n {
		n = len(s)
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		out[i] = c
	}
	return string(out)
}
