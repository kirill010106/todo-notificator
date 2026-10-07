package sender

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/kirill010106/todo-notificator/notifiers/email/internal/config"
)

func TestSender_FromEmailAndSenderName(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("defaults when From and SenderName are empty", func(t *testing.T) {
		s := New(logger, config.SMTP{
			Host:     "smtp-relay.brevo.com",
			Port:     587,
			Username: "bb77b3001@smtp-brevo.com",
			Password: "pass",
		}, nil)

		if s.fromEmail() != "bb77b3001@smtp-brevo.com" {
			t.Errorf("expected fromEmail to fallback to Username, got %s", s.fromEmail())
		}
		if s.senderName() != "ToDoNotificator" {
			t.Errorf("expected default senderName ToDoNotificator, got %s", s.senderName())
		}
	})

	t.Run("uses custom From and SenderName", func(t *testing.T) {
		s := New(logger, config.SMTP{
			Host:       "smtp-relay.brevo.com",
			Port:       587,
			Username:   "bb77b3001@smtp-brevo.com",
			Password:   "pass",
			From:       "notifications@mydomain.com",
			SenderName: "Task Notifier",
		}, nil)

		if s.fromEmail() != "notifications@mydomain.com" {
			t.Errorf("expected custom fromEmail, got %s", s.fromEmail())
		}
		if s.senderName() != "Task Notifier" {
			t.Errorf("expected custom senderName, got %s", s.senderName())
		}
	})
}

func TestSender_BuildMessage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := New(logger, config.SMTP{
		Host:       "smtp-relay.brevo.com",
		Port:       587,
		Username:   "bb77b3001@smtp-brevo.com",
		Password:   "pass",
		From:       "notifications@example.com",
		SenderName: "ToDo Notificator",
	}, nil)

	msg := s.buildMessage("user@example.com", "⏰ Напоминание о дедлайне", "<h1>Привет!</h1>")
	msgStr := string(msg)

	if !strings.Contains(msgStr, "To: user@example.com\r\n") {
		t.Errorf("message missing To header: %s", msgStr)
	}
	if !strings.Contains(msgStr, "notifications@example.com") {
		t.Errorf("message missing from address: %s", msgStr)
	}
	if !strings.Contains(msgStr, "MIME-Version: 1.0\r\n") {
		t.Errorf("message missing MIME-Version header: %s", msgStr)
	}
	if !strings.Contains(msgStr, "Content-Type: text/html; charset=\"UTF-8\"\r\n") {
		t.Errorf("message missing Content-Type header: %s", msgStr)
	}
	if !strings.Contains(msgStr, "<h1>Привет!</h1>") {
		t.Errorf("message missing body: %s", msgStr)
	}
	if !bytes.Contains(msg, []byte("\r\n\r\n<h1>Привет!</h1>")) {
		t.Errorf("message headers and body must be separated by CRLFCRLF")
	}
}
