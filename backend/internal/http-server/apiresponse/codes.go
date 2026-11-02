package apiresponse

// Code is a stable client-facing identifier, independent of the message text.
type Code string

// Existing wire values. Extend this catalog as handlers migrate; do not rename
// codes already consumed by clients. Domain packages do not depend on this one.
const (
	ValidationError         Code = "VALIDATION_ERROR"
	Unauthorized            Code = "UNAUTHORIZED"
	Forbidden               Code = "FORBIDDEN"
	NotFound                Code = "NOT_FOUND"
	MethodNotAllowed        Code = "METHOD_NOT_ALLOWED"
	Conflict                Code = "CONFLICT"
	RateLimited             Code = "RATE_LIMITED"
	InternalError           Code = "INTERNAL_ERROR"
	AccountUnavailable      Code = "ACCOUNT_UNAVAILABLE"
	CategoryLimitReached    Code = "CATEGORY_LIMIT_REACHED"
	BreakAlreadyUsed        Code = "BREAK_ALREADY_USED"
	ReminderLimitReached    Code = "REMINDER_LIMIT_REACHED"
	ReminderValidationError Code = "REMINDER_VALIDATION_ERROR"
	InvalidTaskRequest      Code = "INVALID_TASK_REQUEST"
	InvalidIdempotencyKey   Code = "INVALID_IDEMPOTENCY_KEY"
	IdempotencyKeyReused    Code = "IDEMPOTENCY_KEY_REUSED"
	BodyTooLarge            Code = "BODY_TOO_LARGE"
	PayloadTooLarge         Code = "PAYLOAD_TOO_LARGE"
	EmailExists             Code = "EMAIL_EXISTS"
)
