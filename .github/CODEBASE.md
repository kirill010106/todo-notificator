# Codebase Overview — toDoNotificator

> Last updated: 2026-02-20

## 1. Repository Layout

```
.
├── backend/                   # Go HTTP API
│   ├── cmd/main.go            # Entrypoint
│   ├── config/local.yaml      # App config (YAML)
│   ├── migrations/            # SQL migrations (goose)
│   │   └── 00001_init_db.sql
│   ├── migrations.go          # embed.FS for goose
│   └── internal/
│       ├── config/config.go
│       ├── domain/
│       │   ├── task.go
│       │   ├── token.go       # RefreshToken struct
│       │   └── user.go
│       ├── http-server/
│       │   ├── handlers/
│       │   │   ├── auth/
│       │   │   │   ├── login/login.go
│       │   │   │   ├── logout/logout.go
│       │   │   │   ├── refresh/rerfresh.go   ⚠️ filename typo
│       │   │   │   └── register/register.go
│       │   │   └── health/health.go
│       │   ├── middleware/auth/auth.go
│       │   └── tasks/
│       │       ├── delete/delete.go
│       │       ├── get/get.go
│       │       ├── save/save.go
│       │       └── update/update.go
│       ├── lib/
│       │   ├── api/response/response.go
│       │   ├── handlers/slogpretty.go
│       │   ├── jwt/jwt.go
│       │   └── sl/sl.go
│       └── storage/
│           ├── storage.go        # sentinel errors
│           └── postgres/postgres.go
├── frontend/
│   └── index.html              # Single-file SPA (Alpine.js + Tailwind CDN)
├── telegram-bot/
│   └── cmd/main.go             # Minimal telebot.v4 skeleton
├── go.mod
├── go.sum
├── .env.example
└── .gitignore
```

---

## 2. Tech Stack

| Layer       | Technology                                           |
|-------------|------------------------------------------------------|
| Language    | Go 1.25                                              |
| HTTP Router | `github.com/go-chi/chi/v5` v5.2.4                    |
| HTTP Render | `github.com/go-chi/render`                           |
| CORS        | `github.com/go-chi/cors` v1.2.2                      |
| Auth        | `github.com/golang-jwt/jwt/v5` (HS256)               |
| Password    | `golang.org/x/crypto/bcrypt`                         |
| Database    | PostgreSQL via `github.com/jackc/pgx/v5/stdlib`      |
| DB Errors   | `github.com/jackc/pgerrcode`                         |
| Migrations  | `github.com/pressly/goose/v3` + `embed.FS`           |
| Config      | `github.com/ilyakaznacheev/cleanenv` + YAML + godotenv |
| Validation  | `github.com/go-playground/validator/v10`             |
| Logging     | `log/slog` + custom pretty handler                   |
| Telegram    | `gopkg.in/telebot.v4` (skeleton only)                |
| Frontend    | Alpine.js 3.x + Tailwind CSS v3 (both via CDN)       |

Go module: `github.com/kirill010106/todo-notificator`

---

## 3. Environment & Configuration

### Required env variables (see `.env.example`)

| Variable       | Description                                |
|----------------|--------------------------------------------|
| `DATABASE_URL` | PostgreSQL connection string               |
| `CONFIG_PATH`  | Path to YAML config, e.g. `./config/local.yaml` |
| `APP_SECRET`   | JWT signing secret (min 32 chars)          |
| `BOT_TOKEN`    | Telegram bot token (telegram-bot only)     |

### `config/local.yaml`

```yaml
env: "local"
http_server:
  address: "localhost:8082"
  timeout: 4s
  idle_timeout: 60s
access_token_ttl: 15m
refresh_token_ttl: 168h
```

### `internal/config/config.go` — `Config` struct

```go
type Config struct {
    Env             string        `yaml:"env"`
    StoragePath     string        `yaml:"storage_path"`
    HTTPServer      `yaml:"http_server"`
    AccessTokenTTL  time.Duration `yaml:"access_token_ttl"  env-default:"15m"`
    RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl" env-default:"168h"`
    AppSecret       string        `yaml:"app_secret" env-required:"true" env:"APP_SECRET"`
}
```

> ⚠️ `access_token_ttl` / `refresh_token_ttl` must be at the **root** YAML level, not under `http_server`. Go's `time.Duration` does **not** parse the `d` suffix — use `24h` for one day.

---

## 4. Database Schema

### `users`
| Column          | Type                        | Notes          |
|-----------------|-----------------------------|----------------|
| `id`            | `SERIAL PRIMARY KEY`        |                |
| `email`         | `VARCHAR(255) UNIQUE`       |                |
| `password_hash` | `BYTEA`                     | bcrypt         |
| `telegram_id`   | `BIGINT`                    | nullable       |
| `created_at`    | `TIMESTAMPTZ DEFAULT now()` |                |

### `tasks`
| Column        | Type                          | Notes                          |
|---------------|-------------------------------|--------------------------------|
| `id`          | `SERIAL PRIMARY KEY`          |                                |
| `user_id`     | `INTEGER REFERENCES users(id)`| `ON DELETE CASCADE`            |
| `title`       | `VARCHAR(255) NOT NULL`       |                                |
| `description` | `TEXT NOT NULL`               |                                |
| `deadline`    | `TIMESTAMP`                   | nullable                       |
| `status`      | `task_status DEFAULT 'pending'`| enum: `pending`, `done`       |
| `is_notified` | `BOOLEAN DEFAULT false`       |                                |
| `created_at`  | `TIMESTAMPTZ DEFAULT now()`   |                                |
| `updated_at`  | `TIMESTAMPTZ DEFAULT now()`   |                                |

### `refresh_tokens`
| Column      | Type                         | Notes           |
|-------------|------------------------------|-----------------|
| `id`        | `BIGSERIAL PRIMARY KEY`      |                 |
| `user_id`   | `BIGINT REFERENCES users(id)`| `ON DELETE CASCADE` |
| `token`     | `VARCHAR(64) UNIQUE`         | 32-byte hex     |
| `expires_at`| `TIMESTAMP NOT NULL`         |                 |
| `created_at`| `TIMESTAMP DEFAULT NOW()`    |                 |

**Indexes:** `idx_users_email`, `idx_tasks_user_id`, `idx_tasks_status`, `idx_tasks_deadline`, `idx_refresh_tokens_token`, `idx_refresh_tokens_user_id`, `idx_refresh_tokens_expires_at`

> ⚠️ Known issue: `refresh_tokens.user_id` is `BIGINT` but `users.id` is `SERIAL` (`INTEGER`). FK type mismatch — may cause issues on strict PostgreSQL versions.

---

## 5. API Reference

Base path: `/api/v1`  
Server default: `http://localhost:8082`

### Public endpoints

| Method | Path        | Handler          | Description              |
|--------|-------------|------------------|--------------------------|
| `POST` | `/register` | `register.New`   | Create user              |
| `POST` | `/login`    | `login.New`      | Returns access + refresh token |
| `POST` | `/refresh`  | `refresh.New`    | Token rotation           |
| `GET`  | `/health`   | `health.New`     | DB ping check            |

### Protected endpoints (require `Authorization: Bearer <access_token>`)

| Method   | Path              | Handler        | Description           |
|----------|-------------------|----------------|-----------------------|
| `POST`   | `/tasks`          | `save.New`     | Create task           |
| `GET`    | `/tasks`          | `get.New`      | List tasks (no pagination currently) |
| `PATCH`  | `/tasks/{task_id}`| `update.New`   | Partial update        |
| `DELETE` | `/tasks/{task_id}`| `delete.New`   | Delete task           |
| `POST`   | `/logout`         | `logout.New`   | Revoke refresh token  |

---

## 6. Authentication Flow

```
POST /login
  → bcrypt verify
  → generate access token (JWT HS256, claims: uid, email, type="access")
  → generate refresh token (crypto/rand 32 bytes → hex string)
  → save refresh token to DB with expires_at
  → return { access_token, refresh_token, expires_in }

Protected request
  → middleware reads Authorization: Bearer <token>
  → validates JWT (algorithm check, type="access", valid)
  → sets user_id in context via typed key (contextKey("user_id"))
  → handler calls auth.GetUserID(ctx)

POST /refresh  { refresh_token }
  → lookup token in DB (WHERE token=$1 AND expires_at > NOW())
  → get user by user_id
  → delete old token
  → generate new access + refresh token pair
  → save new refresh token
  → return new pair

POST /logout   { refresh_token, all_devices: bool }
  → if all_devices=true → DeleteUserRefreshTokens(userID)
  → else → DeleteRefreshToken(token)
```

---

## 7. Domain Models

### `domain.Task`
```go
type Task struct {
    ID          int64      `json:"id"`
    UserID      int64      `json:"user_id"`
    Title       string     `json:"title"`
    Description string     `json:"description"`
    Deadline    *time.Time `json:"deadline,omitempty"`
    Status      string     `json:"status"`           // "pending" | "done"
    IsNotified  bool       `json:"is_notified"`
}
```

### `domain.TaskUpdate` (partial update, all pointer fields)
```go
type TaskUpdate struct {
    Title       *string
    Description *string
    Status      *string
    Deadline    *time.Time
}
```

### `domain.User`
```go
type User struct {
    ID       int64  `json:"id"`
    Email    string `json:"email"`
    PassHash []byte `json:"-"`
}
```

### `domain.RefreshToken` (in `domain/token.go`)
```go
type RefreshToken struct {
    ID        int64     `json:"-"`
    UserID    int64     `json:"user_id"`
    Token     string    `json:"token"`
    ExpiresAt time.Time `json:"expires_at"`
    CreatedAt time.Time `json:"created_at"`
}
```

---

## 8. Storage Layer (`postgres.Storage`)

All methods require a `context.Context` as first argument.

| Method                                                    | Description                           |
|-----------------------------------------------------------|---------------------------------------|
| `SaveTask(ctx, task) (int64, error)`                      | INSERT, returns new ID                |
| `GetTasks(ctx, userID int64) ([]Task, error)`             | SELECT all tasks for user             |
| `DeleteTask(ctx, userID, taskID int64) error`             | DELETE WHERE user_id AND id           |
| `UpdateTask(ctx, userID, taskID int64, t TaskUpdate) error` | Dynamic SET, uses RowsAffected      |
| `User(ctx, email) (User, error)`                          | Lookup by email                       |
| `SaveUser(ctx, email, passHash) (int64, error)`           | INSERT user                           |
| `GetUserByID(ctx, userID) (*User, error)`                 | Lookup by ID                          |
| `SaveRefreshToken(ctx, userID, token, expiresAt) error`   | INSERT refresh token                  |
| `GetRefreshToken(ctx, token) (*RefreshToken, error)`      | SELECT WHERE token AND not expired    |
| `DeleteRefreshToken(ctx, token) error`                    | DELETE single token                   |
| `DeleteUserRefreshTokens(ctx, userID) error`              | DELETE all tokens for user            |

**Sentinel errors** (`internal/storage/storage.go`):
- `storage.ErrTaskNotFound`
- `storage.ErrTaskExists`
- `storage.ErrUserNotFound`
- `storage.ErrUserExists`
- `storage.ErrRefreshTokenInvalid`

---

## 9. JWT Library (`internal/lib/jwt`)

```go
// NewAccessToken — creates HS256 JWT with claims: uid, email, type="access", exp, iat
func NewAccessToken(user domain.User, secret string, duration time.Duration) (string, error)

// NewRefreshToken — generates 32 random bytes encoded as 64-char hex string
func NewRefreshToken() (string, error)

// ParseAccessToken — validates algorithm (HMAC only), token type, returns uid
func ParseAccessToken(tokenString, secret string) (int64, error)
```

---

## 10. Auth Middleware (`internal/http-server/middleware/auth`)

```go
// New — chi middleware: parses Bearer token, puts user_id into context
func New(secret string) func(next http.Handler) http.Handler

// GetUserID — type-safe context extraction (uses typed key, not plain string)
func GetUserID(ctx context.Context) (int64, bool)
```

Context key is a private `type contextKey string` — prevents collisions with other packages.

---

## 11. Response Helpers (`internal/lib/api/response`)

```go
type Response struct {
    Status string `json:"status"`
    Error  string `json:"error,omitempty"`
}

func OK() Response             // status: "OK"
func Error(msg string) Response // status: "Error", error: msg
func ValidationError(errs validator.ValidationErrors) Response
```

---

## 12. CORS Configuration

All origins allowed (`*`), methods: `GET POST PUT PATCH DELETE OPTIONS`, headers: `Accept Authorization Content-Type`. `AllowCredentials: false`, `MaxAge: 300`.

> For production, replace `AllowedOrigins: []string{"*"}` with specific domain(s).

---

## 13. Frontend (`frontend/index.html`)

Single HTML file, no build step. Uses CDN:
- Alpine.js 3.x (state management)
- Tailwind CSS v3 (styling)

**Features:**
- Auth: Login / Register tabs, show/hide password, password strength indicator
- Dashboard: task list, filter buttons (all / pending / done with counts)
- Create task: title, description, `datetime-local` deadline
- Inline edit: title, description, deadline, status dropdown
- Checkbox behavior: **if task is being edited** → updates `editForm.status` locally (no API call); only saves on "Save" button. If not editing → calls `toggleTaskStatus()` immediately.
- Status badge reflects local `editForm.status` while editing
- Pagination: prev/next, "Page X of Y" — **note:** backend `GET /tasks` does not yet support `limit`/`offset` query params
- Automatic token refresh on 401 (retry with new access token)
- Toast notifications (success / error / info, 3 s timeout)
- Logout with refresh token revocation

**API_BASE:** `http://localhost:8082/api/v1`

---

## 14. Telegram Bot (`telegram-bot/cmd/main.go`)

Minimal skeleton. Reads `BOT_TOKEN` from env, registers `/hello` handler, uses `LongPoller` (10 s timeout). No integration with the backend yet.

---

## 15. Migrations

Managed by [goose](https://github.com/pressly/goose) v3.  
SQL files are embedded at compile time via `backend/migrations.go`:

```go
//go:embed migrations/*.sql
var MigrationsFS embed.FS
```

`goose.Up` is called automatically on server start in `main.go`.

Current migration: `00001_init_db.sql`  
Creates: `task_status` enum, `users`, `tasks`, `refresh_tokens` tables + indexes.

---

## 16. Known Issues & TODOs

| # | Location | Issue |
|---|----------|-------|
| 1 | `refresh/rerfresh.go` | **Filename typo** (`rerfresh.go` instead of `refresh.go`) |
| 2 | `postgres.go:GetUserByID` | **SQL bug**: query uses `WHERE user_id = $1` — column is `id`, not `user_id`. Will always return `ErrUserNotFound`. |
| 3 | `migration` | `refresh_tokens.user_id` is `BIGINT`, `users.id` is `SERIAL` (`INTEGER`) — FK type mismatch |
| 4 | `postgres.go:UpdateTask` | Does **not** update the `updated_at` column despite the column existing in the schema |
| 5 | `get.go` | No pagination implemented — `GetTasks` takes only `userID`, no `limit`/`offset`. Frontend pagination controls are non-functional. |
| 6 | `rerfresh.go` | Missing `return` after 401 response in `GetRefreshToken` error branch — falls through to 500 response |
| 7 | `main.go` | No graceful shutdown (`signal.NotifyContext` + `srv.Shutdown`) |
| 8 | `main.go` | `goose.Up` failure only logs a warning (no `os.Exit`) — server starts with potentially missing schema |
| 9 | General | No rate limiting |
| 10 | General | No `http.MaxBytesReader` on request bodies |

---

## 17. Running Locally

```powershell
# 1. Copy and fill env
Copy-Item .env.example .env
# edit .env with your DATABASE_URL and APP_SECRET

# 2. Run the server (migrations apply automatically)
cd backend
go run ./cmd/main.go

# 3. Open the frontend
start ..\frontend\index.html
```

---

## 18. Building

```bash
go build ./backend/...
```

Clean build verified ✓ (exit 0, no errors).
