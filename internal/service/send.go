package service

import (
	"fmt"
	"strings"

	"learn/smtp-platform/internal/models"
	"learn/smtp-platform/internal/repo"
)

type SendService struct {
	events *repo.MailEventRepo
	queue  *repo.QueueRepo
}

func NewSendService(users *repo.UserRepo, domains *repo.DomainThrottleRepo, events *repo.MailEventRepo, queue *repo.QueueRepo, limiter *Limiter) *SendService {
	return &SendService{events: events, queue: queue}
}

func (s *SendService) HandleMail(user models.User, from string, to []string, data []byte) error {
	for _, rcpt := range to {
		domain := extractDomain(rcpt)
		if domain == "" {
			s.events.Create(models.MailEvent{UserID: user.ID, MailFrom: from, RcptTo: rcpt, Domain: "", Status: "rejected", Reason: "invalid recipient domain"})
			return fmt.Errorf("invalid recipient %s", rcpt)
		}

		if err := s.queue.Enqueue(models.QueueItem{UserID: user.ID, MailFrom: from, RcptTo: rcpt, Data: data}); err != nil {
			s.events.Create(models.MailEvent{UserID: user.ID, MailFrom: from, RcptTo: rcpt, Domain: domain, Status: "failed", Reason: "queue insert failed"})
			return err
		}

		s.events.Create(models.MailEvent{UserID: user.ID, MailFrom: from, RcptTo: rcpt, Domain: domain, Status: "queued", Reason: ""})
	}
	return nil
}

func extractDomain(addr string) string {
	parts := strings.Split(strings.TrimSpace(addr), "@")
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(parts[1])
}
