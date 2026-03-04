package smtp

import (
	"errors"
	"io"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
	"learn/smtp-platform/internal/config"
	"learn/smtp-platform/internal/models"
	"learn/smtp-platform/internal/service"
)

type Backend struct {
	auth *service.AuthService
	send *service.SendService
}

func NewServer(cfg config.Config, auth *service.AuthService, send *service.SendService) *gosmtp.Server {
	b := &Backend{auth: auth, send: send}
	s := gosmtp.NewServer(b)
	s.Addr = cfg.SMTPListenAddr
	s.Domain = "localhost"
	s.AllowInsecureAuth = true
	s.MaxRecipients = 100
	s.MaxMessageBytes = 20 * 1024 * 1024
	s.ReadTimeout = 30 * time.Second
	s.WriteTimeout = 30 * time.Second
	return s
}

func (b *Backend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	return &Session{backend: b}, nil
}

type Session struct {
	backend *Backend
	user    models.User
	from    string
	to      []string
}

func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain, "LOGIN"}
}

func (s *Session) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			u, err := s.backend.auth.Validate(username, password)
			if err != nil {
				return err
			}
			s.user = u
			return nil
		}), nil
	case "LOGIN":
		var username string
		return sasl.NewLoginServer(func(user, pass string) error {
			if username == "" {
				username = user
			}
			u, err := s.backend.auth.Validate(username, pass)
			if err != nil {
				return err
			}
			s.user = u
			return nil
		}), nil
	default:
		return nil, errors.New("unsupported auth mechanism")
	}
}

func (s *Session) Mail(from string, _ *gosmtp.MailOptions) error {
	if s.user.ID == 0 {
		return errors.New("auth required")
	}
	s.from = from
	s.to = nil
	return nil
}

func (s *Session) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	if s.user.ID == 0 {
		return errors.New("auth required")
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return s.backend.send.HandleMail(s.user, s.from, s.to, body)
}

func (s *Session) Reset() {}
func (s *Session) Logout() error { return nil }