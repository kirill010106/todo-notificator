package sender

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/email/internal/config"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/formatter"
	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
)

type Sender struct {
	log       *slog.Logger
	cfg       config.SMTP
	formatter *formatter.Formatter
}

func New(log *slog.Logger, cfg config.SMTP, formatter *formatter.Formatter) *Sender {
	return &Sender{
		log:       log,
		cfg:       cfg,
		formatter: formatter,
	}
}

func (s *Sender) Send(user domain.User, task domain.Task, interval time.Duration) error {
	const op = "sender.Send"
	body, err := s.formatter.Format(task, interval)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	subject := s.formatter.Subject(task, interval)

	if err := s.send(user.Email, subject, body); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	s.log.Info("email sent",
		slog.String("to", user.Email),
		slog.Int64("task_id", task.ID),
		slog.Duration("interval", interval),
	)
	return nil
}

func (s *Sender) send(to, subject, body string) error {
	const op = "sender.send"

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	msg := s.buildMessage(to, subject, body)

	if s.cfg.Port == 465 {
		return s.sendWithTLS(addr, auth, to, msg)
	}

	return s.sendWithSTARTTLS(addr, auth, to, msg)
}

func (s *Sender) sendWithTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	const op = "sender.sendWithTLS"

	tlsConfig := &tls.Config{
		ServerName: s.cfg.Host,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("%s dial: %w", op, err)
	}

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("%s client: %w", op, err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("%s auth: %w", op, err)
	}

	if err := client.Mail(s.cfg.Username); err != nil {
		return fmt.Errorf("%s mail from: %w", op, err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("%s rcpt to: %w", op, err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("%s data: %w", op, err)
	}
	defer w.Close()

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("%s write: %w", op, err)
	}

	return nil
}

func (s *Sender) sendWithSTARTTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	const op = "sender.sendWithSTARTTLS"

	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("%s dial: %w", op, err)
	}
	defer client.Close()

	tlsConfig := &tls.Config{
		ServerName: s.cfg.Host,
	}

	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("%s starttls: %w", op, err)
	}

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("%s auth: %w", op, err)
	}

	if err := client.Mail(s.cfg.Username); err != nil {
		return fmt.Errorf("%s mail from: %w", op, err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("%s rcpt to: %w", op, err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("%s data: %w", op, err)
	}
	defer w.Close()

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("%s write: %w", op, err)
	}

	return nil
}

// buildMessage собирает raw email сообщение
func (s *Sender) buildMessage(to, subject, body string) []byte {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("From: ToDoNotificator <%s>\r\n", s.cfg.Username))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", to))
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)

	return []byte(sb.String())
}

func (s *Sender) SendVerificationEmail(email, token string) error {
	const op = "sender.SendVerificationEmail"

	subject := "Подтверждение регистрации ToDoNotificator"
	body, err := s.formatter.Verification(token)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.send(email, subject, body); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	s.log.Info("verification email sent",
		slog.String("to", email),
	)
	return nil
}
