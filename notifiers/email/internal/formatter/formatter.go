package formatter

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/shared/domain"
)

//go:embed templates/deadline.html templates/verification.html
var templatesFS embed.FS

type deadlineTemplateData struct {
	Task              domain.Task
	UrgencyText       string
	BadgeClass        string
	DeadlineFormatted string
}

type verificationTemplateData struct {
	VerificationLink string
}

type Formatter struct {
	deadlineTmpl     *template.Template
	verificationTmpl *template.Template
	appURL           string
}

func New(appURL string) (*Formatter, error) {
	deadlineTmpl, err := template.ParseFS(templatesFS, "templates/deadline.html")
	if err != nil {
		return nil, fmt.Errorf("formatter.New: failed to parse deadline template: %w", err)
	}

	verificationTmpl, err := template.ParseFS(templatesFS, "templates/verification.html")
	if err != nil {
		return nil, fmt.Errorf("formatter.New: failed to parse verification template: %w", err)
	}

	return &Formatter{
		deadlineTmpl:     deadlineTmpl,
		verificationTmpl: verificationTmpl,
		appURL:           appURL,
	}, nil
}

func (f *Formatter) Format(task domain.Task, interval time.Duration) (string, error) {
	data := deadlineTemplateData{
		Task:              task,
		UrgencyText:       formatUrgencyText(interval),
		BadgeClass:        formatBadgeClass(interval),
		DeadlineFormatted: formatDeadline(task.Deadline),
	}

	var buf bytes.Buffer
	if err := f.deadlineTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("formatter.Format (deadline): %w", err)
	}

	return buf.String(), nil
}

// Verification generates the HTML body for the verification email
func (f *Formatter) Verification(token string) (string, error) {
	link := fmt.Sprintf("%s/?token=%s", f.appURL, token)

	data := verificationTemplateData{
		VerificationLink: link,
	}

	var buf bytes.Buffer
	if err := f.verificationTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("formatter.Verification: %w", err)
	}

	return buf.String(), nil
}

func (f *Formatter) Subject(task domain.Task, interval time.Duration) string {
	return fmt.Sprintf("⏰ %s — %s", formatUrgencyText(interval), task.Title)
}

func formatUrgencyText(interval time.Duration) string {
	switch {
	case interval >= 24*time.Hour:
		days := int(interval.Hours() / 24)
		return fmt.Sprintf("До дедлайна %d %s", days, pluralDay(days))
	case interval >= time.Hour:
		hours := int(interval.Hours())
		return fmt.Sprintf("До дедлайна %d %s", hours, pluralHour(hours))
	default:
		minutes := int(interval.Minutes())
		return fmt.Sprintf("До дедлайна %d %s!", minutes, pluralMinute(minutes))
	}
}

func formatBadgeClass(interval time.Duration) string {
	switch {
	case interval <= 30*time.Minute:
		return "urgent"
	case interval <= 3*time.Hour:
		return "warning"
	default:
		return "normal"
	}
}

func formatDeadline(deadline *time.Time) string {
	if deadline == nil {
		return "не указан"
	}
	return deadline.Format("02.01.2006 в 15:04")
}

func pluralDay(n int) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return "день"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20):
		return "дня"
	default:
		return "дней"
	}
}

func pluralHour(n int) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return "час"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20):
		return "часа"
	default:
		return "часов"
	}
}

func pluralMinute(n int) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return "минута"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20):
		return "минуты"
	default:
		return "минут"
	}
}
