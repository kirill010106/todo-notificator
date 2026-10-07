# Codebase Audit — toDoNotificator

> **Last updated:** 2026-02-20 (rewritten)
> **Revision audited:** branch `feature/backend`, HEAD `5836d61` + 48 uncommitted files (`+3611 / −877`)
> **Verification:** `go build ./...` → exit 0; `go test ./... -count=1` → green in backend, notifiers/shared, notifiers/email. `activity-logger` integration test fails **only** because rootless Docker is unavailable on this Windows host (CI on ubuntu-latest is unaffected).
> **Scope of this document:** repository and system audit — layout, stack, config, schema, API, data flows, test coverage, and verified defects. It replaces the previous version, which described a much smaller project (3 handlers, no payments, no gamification, no activity-logger, no notifiers, 2 migrations).

**Scale:** ~6,900 lines of Go across 160 files · 41 test files / 192 test functions · 15 migrations · Alpine.js SPA 3,708 lines · React client 1,207 lines TS/TSX · OpenAPI spec 1,293 lines

---

## 1. Quick reference

| Question | Answer |
|---|---|
| What is this? | Task manager with email reminders, Pomodoro timer, gamification, premium subscriptions |
| Shape | Go modular monorepo: 1 HTTP API + 2 Go services (gRPC logger, email notifier) + 2 web clients |
| Storage | PostgreSQL (transactional), MongoDB (append-only activity log) |
| Language | Go 1.25 (toolchain `go.work`), TypeScript for React client, plain JS for Alpine client |
| Deployment | Docker → GHCR → Ansible → server behind Traefik with TLS |
| Live hosts | `todo.chkpnk.ru` (web), `todoapi.chkpnk.ru` (API) — see `docker-compose.yml:50,85` |
| Healthiest area | Backend test suite: 41 files, 192 tests, all green |
| Weakest area | Observability (zero metrics/tracing/profiling) and payment-webhook idempotency |
| Biggest doc risk | `docs/openapi.yaml` documents 20 operations; the router exposes 32 |

---

## 2. Repository layout (actual)

```
toDoNotificator/
├── go.work                   # workspace: ., ./activity-logger, ./backend, ./notifiers, ./notifiers/email
├── go.mod                    # module github.com/kirill010106/todo-notificator/root  ← holds pkg/ + migrations embed
├── docker-compose.yml        # mongo, activity-logger, backend, email-notifier, frontend (+external traefik net)
├── dev.ps1                   # local launcher: backend + email notifier via air
├── webhook.ps1               # manual webhook trigger helper
│
├── backend/                  # module github.com/kirill010106/todo-notificator  (main HTTP API)
├── activity-logger/          # module activity-logger  (gRPC + MongoDB)
├── notifiers/
│   ├── go.mod                # module .../notifiers  (shared + telegram-bot)
│   ├── shared/               # domain, storage (PostgreSQL, SELECT-only), scheduler
│   ├── email/                # module .../notifiers/email  (SMTP sender, formatter, webhook)
│   └── telegram-bot/         # frozen skeleton
├── proto/activity_logger/v1/activity_logger.proto   # source of truth for the gRPC contract
├── pkg/activity_logger/v1/   # generated *.pb.go  ← belongs to the ROOT module
├── frontend/                 # DEPLOYED client: Dockerfile + index.html (3,708 lines, Alpine.js + Tailwind CDN)
├── frontend-churka/todo-notificator/   # alternate React 19 client (Vite, mobx, react-query, antd) — not deployed
├── docs/                     # openapi.yaml (1,293 lines), Swagger UI express server, ROADMAP-ADVANCED.md
├── ansible/                  # deploy.yml + inventory.ini
└── .github/workflows/        # ci.yml, cd.yml
```

> ⚠️ **Two undeclared modules / three naming styles.** `go.work` lists five modules but not the root `go.mod`, so `./backend/clients/activitylogger` and `./activity-logger` reach the generated protobuf code by importing `github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1`. Working, but the `root` module name and the `replace … => ../` in `activity-logger/go.mod:47` are the kind of thing a reviewer notices first.

---

## 3. Tech stack

### Backend (`backend/go.mod`)

| Layer | Technology |
|---|---|
| Router | `go-chi/chi/v5` v5.2.4 + `go-chi/render` v1.0.3 |
| Rate limiting | `go-chi/httprate` v0.15.0 — 100 req/min **by IP** |
| CORS | `go-chi/cors` v1.2.2 — configured **in code**, not from YAML |
| Auth | `golang-jwt/jwt/v5` v5.3.1 (HS256), `golang.org/x/crypto` v0.49.0 (bcrypt) |
| Validation | `go-playground/validator/v10` v10.30.1 |
| Database | PostgreSQL via `jackc/pgx/v5` v5.8.0 (`database/sql` stdlib adapter) |
| DB errors | `jackc/pgerrcode` |
| Migrations | `pressly/goose/v3` v3.26.0 + `embed.FS` (`migrations.go`) |
| Config | `ilyakaznacheev/cleanenv` v1.5.0 + YAML + `joho/godotenv` |
| Logging | `log/slog` + custom pretty handler (`internal/lib/handlers/slogpretty.go`) |
| gRPC client | `google.golang.org/grpc` v1.80.0 |
| Payments | `rvinnie/yookassa-sdk-go` v0.1.6 — **listed as `// indirect` although directly imported** |
| Tests | `stretchr/testify`, `DATA-DOG/go-sqlmock`, `testcontainers-go` v0.42.0 (+ postgres module) |

### Other services

| Service | Notable dependencies |
|---|---|
| `activity-logger` | `go.mongodb.org/mongo-driver/v2` v2.5.1, gRPC v1.80.0, testcontainers mongodb |
| `notifiers` (shared) | `lib/pq` v1.11.1 (note: **not pgx**, unlike the backend), `telebot.v4` for the frozen bot |
| `notifiers/email` | only `cleanenv` + `godotenv`; SMTP via stdlib `net/smtp`; templates via `html/template` |

### Clients

| Client | Stack | Deployed? |
|---|---|---|
| `frontend/index.html` | Alpine.js 3 + Tailwind (both CDN), no build step | ✅ via nginx |
| `frontend-churka/todo-notificator` | React 19, Vite 8, TypeScript 6, MobX, TanStack Query 5, Ant Design 6, axios, dayjs, sass | ❌ not in `cd.yml` |

**Interesting:** OpenTelemetry (`otel`, `otelhttp`, `otel/sdk`, `otel/trace`) is already in `backend` and `activity-logger` dependency graphs as `// indirect`, but **zero instrumentation call sites exist**. Tracing infrastructure is pre-installed and unused.

---

## 4. Configuration

### Env vars

| Variable | Consumer | Required |
|---|---|---|
| `DATABASE_URL` | backend, email notifier | yes |
| `CONFIG_PATH` | backend (`config.MustLoad`) | yes |
| `APP_SECRET` | backend JWT signing, min 32 chars | yes (`env-required`) |
| `EMAIL_CONFIG_PATH` | email notifier (falls back to `config/local.yaml`) | no |
| `WEBHOOK_URL` / `WEBHOOK_SECRET` | backend → notifier; also read by notifier | secret yes for notifier |
| `SMTP_HOST/PORT/USERNAME/PASSWORD/FROM/SENDER_NAME` | email notifier | username+password yes |
| `SHOP_ID` / `SECRET_KEY` | backend YooKassa | no (payments disabled if empty) |
| `CLIENT_URL`, `ENV`, `STORAGE_PATH`, `ACTIVITY_LOGGER_ADDR` | backend | no |
| `MONGO_URL` | activity-logger (default `mongodb://localhost:27017`) | no |
| `BOT_TOKEN` | telegram-bot | no (frozen) |

### Config files

- `backend/config/local.yaml` — `env: local`, `http_server: 0.0.0.0:8082`, webhook `localhost:8084`, activity-logger `localhost:50051`, TTLs 15m/168h.
- `backend/config/prod.yaml` — `env: prod`, webhook `http://email-notifier:8084`, activity-logger `activity-logger:50051`, client `https://todo.chkpnk.ru`.
- `notifiers/email/config/{local,prod}.yaml` — SMTP (Brevo relay), `app_url`, webhook address/secret.
- Local secrets live in untracked `backend/.env` and `notifiers/email/.env` — **verified not in git index**.

> 🔴 **Config keys that are silently ignored.** `backend/config/local.yaml:21-36` defines a `cors:` block (`allowed_origins`, `allowed_methods`, `allowed_headers`, `allow_credentials`) that **no Go struct maps**: `config.Config` has no `CORS` field (`backend/internal/config/config.go:11-22`), and `router.go:69-75` hardcodes `AllowedOrigins: []string{"*"}`, `AllowCredentials: false`. Anyone editing that YAML block changes nothing. Same class of problem: `clients.activity_logger.retries_count` is parsed (`config.go:36`) but `backend/cmd/main.go:53` never passes it to `activitylogger.New`, which accepts no retry parameter.
>
> 🔴 **Both `.env.local.example` files are empty.** `backend/.env.local.example` and `notifiers/email/.env.local.example` are tracked in git and contain **zero bytes**. Meanwhile `README.md:112-116` instructs `Copy-Item notifiers/email/deploy/.env.notifier.example notifiers/email/.env` — that `deploy/` directory **does not exist**. A new contributor following the README is blocked twice.

---

## 5. Database schema

### `users`
`id SERIAL PK` · `email VARCHAR(255) UNIQUE` · `password_hash BYTEA` · `telegram_id BIGINT` · `created_at TIMESTAMPTZ` · `is_verified BOOLEAN DEFAULT false` (00012) · `is_premium BOOLEAN DEFAULT false` (00016)

### `tasks`
`id SERIAL PK` · `user_id INTEGER FK→users ON DELETE CASCADE` · `title VARCHAR(255)` · `description TEXT` · `deadline TIMESTAMP` · `status task_status DEFAULT 'pending'` · `is_notified BOOLEAN DEFAULT false` · `created_at` / `updated_at TIMESTAMPTZ` · `reminder_at TIMESTAMP` (00004) · `category_id INTEGER FK→categories` (00005) · `burnt BOOLEAN DEFAULT false` (00007) · `pomodoros_taken` (00011) · `reward_claimed` (00015)

**Enum `task_status`:** `pending`, `done`, **`burnt`** (added later; the Go constant set is in `domain/task.go:5-10`).

### `categories`
`id SERIAL PK` · `user_id BIGINT FK→users` · `name VARCHAR(64)` · `UNIQUE(user_id, name)`

### `refresh_tokens`
`id BIGSERIAL PK` · `user_id BIGINT FK→users` · `token VARCHAR(64) UNIQUE` · `expires_at TIMESTAMP` · `created_at`

### `email_verification_tokens`
`id SERIAL PK` · `user_id INTEGER FK→users` · `token VARCHAR(64) UNIQUE` · `expires_at TIMESTAMPTZ` · `created_at`

### `user_stats` (gamification, 00007 + 00017)
`id SERIAL PK` · `user_id INT UNIQUE FK→users` · `points INT` · `level INT` · `total_pomodoros INT` · `total_burnt_tasks INT` · `current_streak INT` · `best_streak INT` · `last_activity_date DATE` · `updated_at TIMESTAMP`

### `pomodoro_sessions`
`id SERIAL PK` · `user_id INT` · `task_id INT FK→tasks` · `started_at` · `completed_at` · `duration_minutes INT` · `breaks_used INT` · `created_at`

### `achievements` (00017)
`id BIGSERIAL PK` · `user_id BIGINT FK→users` · `code VARCHAR(64)` · `unlocked_at TIMESTAMPTZ` · `UNIQUE(user_id, code)`

### `payments` (00016)
`id SERIAL PK` · `yookassa_payment_id VARCHAR(255) UNIQUE` · `user_id BIGINT FK→users` · `amount DECIMAL(10,2)` · `currency VARCHAR(3)` · `status VARCHAR(50)` · `description TEXT` · `created_at` / `updated_at TIMESTAMPTZ`

### Indexes present
`idx_users_email` · `idx_tasks_user_id` · `idx_tasks_status` · `idx_tasks_deadline` · `idx_tasks_category_id` · `idx_refresh_tokens_{token,user_id,expires_at}` · `idx_email_verification_tokens_{token,user_id}` · `idx_achievements_user_id`

### 🔴 Missing indexes (both on hot paths)

1. **Scheduler query unindexed.** `notifiers/shared/storage/postgres/postgres.go:98-137` filters on `reminder_at`, `is_notified`, `status`. The only related index is a bare `idx_tasks_deadline ON tasks(deadline)` — which is **not the same column** (`reminder_at` was added later in 00004 and never indexed). Every poll is a sequential scan.
2. **No idempotency table for webhooks.** `payments` has no unique key on a webhook *event* identifier, so duplicate YooKassa deliveries cannot be deduplicated at the storage layer.

### Migration numbering gaps
Present: 00001, 00003–00008, 00010–00017 (**15 files**). Missing: **00002, 00009**. Goose tolerates gaps, but the history reads as if files were lost — worth a note in the README to prevent a future "restore the missing migration" mistake.

### ⚠️ Foreign-key type mismatches (carried over, still real)

| Child column | Type | Parent | Type |
|---|---|---|---|
| `refresh_tokens.user_id` | `BIGINT` | `users.id` | `SERIAL` = INTEGER |
| `categories.user_id` | `BIGINT` | `users.id` | `SERIAL` |
| `achievements.user_id` | `BIGINT` | `users.id` | `SERIAL` |
| `payments.user_id` | `BIGINT` | `users.id` | `SERIAL` |
| `email_verification_tokens.user_id` | `INTEGER` ✅ | `users.id` | `SERIAL` |

Five child tables, four of them `BIGINT`, one `INTEGER`. PostgreSQL permits the FK (it widens the comparison), so this is inconsistent rather than broken — but it is exactly the kind of asymmetry a schema reviewer flags, and it will bite whoever migrates `users.id` later.

### ⚠️ Invariants that live only in Go
`user_stats` has **no CHECK constraints** on `level` or `points`, and `CalculateLevel` (`domain/stats.go:92`) is the only thing enforcing the 1–100 cap and non-negative points. Paths that bypass it — e.g. `UpdateUserScore` (`storage/postgres/postgres.go:867`) and `UpdateUserStats` (`:748`) — can write out-of-range values straight to the column.

---

## 6. API surface

Base path `/api/v1`. Server default `:8082` (local config `0.0.0.0:8082`).

### Public (router.go:115-121)

| Method | Path | Handler |
|---|---|---|
| POST | `/register` | `handlers/auth/register` |
| POST | `/login` | `handlers/auth/login` |
| POST | `/refresh` | `handlers/auth/refresh` |
| GET | `/verify` | `handlers/auth/verify` |
| GET | `/health` | `handlers/health` (Postgres ping, 2s timeout) |
| POST | `/webhooks/yookassa` | `handlers/payments/webhook` |

### Protected — `Authorization: Bearer <access_token>` (router.go:124-168)

| Method | Path | Handler |
|---|---|---|
| POST | `/logout` | `handlers/auth/logout` |
| POST | `/verify/resend` | `handlers/auth/resend` |
| GET | `/me/bootstrap` | `handlers/profile/bootstrap` — aggregated initial state |
| GET | `/me/profile` | `handlers/profile/get` |
| GET | `/me/achievements` | `handlers/profile/achievements` |
| GET | `/me/quests` | `handlers/profile/quests` |
| GET | `/me/logs` | `handlers/logs/get` → gRPC activity-logger |
| GET · PATCH | `/me/stats` | `handlers/stats/{get,update}` |
| GET · POST | `/tasks` | `handlers/tasks/{get,save}` |
| PATCH · DELETE | `/tasks/{task_id}` | `handlers/tasks/{update,delete}` |
| POST | `/tasks/bulk-complete` | `handlers/tasks/bulk` |
| POST | `/tasks/bulk-delete` | `handlers/tasks/bulk` |
| GET · POST | `/categories` | `handlers/categories/{get,create}` |
| GET · PATCH · DELETE | `/categories/{category_id}` | `handlers/categories/{getone,update,delete}` |
| POST | `/payments/create` | `handlers/payments/create` |
| POST | `/payments/sync` | `handlers/payments/sync` |
| POST | `/pomodoros/start` | `handlers/pomodoros/start` |
| GET | `/pomodoros/active` | `handlers/pomodoros/active` |
| POST | `/pomodoros/{id}/pause` | `handlers/pomodoros/pause` |
| POST | `/pomodoros/{id}/stop` | `handlers/pomodoros/stop` |
| POST | `/dev/toggle-premium` | `handlers/dev/premium` — **registered only when `Env == "local"`** (router.go:165) |

### Non-API roots
`GET /` (service banner JSON) · `GET /ping` (plain `pong`). Both excluded from request logging (`middleware/logger/logger.go:19`).

### Task listing query parameters (implemented — `handlers/tasks/get/get.go`)

`limit` (default 20, max 100, clamped) · `offset` · `status` (`all|pending|done|burnt`) · `search` (ILIKE on title+description) · `category_id` · `sort_by` (`created_at` default, `deadline`) · `order` (`desc` default, `asc`). Response includes a `pagination{limit,offset,total}` object.

### 🔴 OpenAPI drift — 12 of 32 operations undocumented

`docs/openapi.yaml` (v1.2.0) covers 20 operations and **omits**:

`GET /me/bootstrap` · `GET /me/profile` · `GET /me/achievements` · `GET /me/quests` · `POST /tasks/bulk-complete` · `POST /tasks/bulk-delete` · `POST /payments/create` · `POST /payments/sync` · `POST /webhooks/yookassa` · `POST /dev/toggle-premium` · `GET /` · `GET /ping`

Three sources of truth — `router.go`, `README.md`, `openapi.yaml` — disagree about paths and about which endpoints are public. **Only `router.go` is authoritative**; `README.md` still lists the pre-categories/payments/pomodoro endpoint set.

---

## 7. Authentication & authorization

```
POST /register  → bcrypt hash → INSERT users → save verification token → webhook "verification" to notifier
POST /login     → bcrypt compare → access (HS256 JWT) + refresh (32 random bytes → 64-char hex) → store refresh
GET  /verify    → token lookup → VerifyUserEmail → delete token
POST /refresh   → RotateRefreshToken (single DB call: validate + delete old + insert new) → new pair
POST /logout    → DeleteRefreshToken, or DeleteUserRefreshTokens when all_devices
```

**Access token claims** (`lib/jwt/jwt.go:13-25`): `uid`, `email`, `type="access"`, `exp`, `iat`, `is_verified`, `is_premium`.
**Validation** (`jwt.go:42-83`): HMAC-family algorithm check, `type == "access"`, then manual extraction of `uid` (as `float64`), `is_verified`, `is_premium`.
**Middleware** (`middleware/auth/auth.go:19-52`): strict `Bearer <token>` shape (exactly 2 parts), then stores **three** typed context values via an unexported `contextKey` type: `user_id`, `is_premium`, `is_verified`. Accessors: `GetUserID`, `GetPremiumStatus`, `GetVerificationStatus`.
**Handler helper** (`helpers/logger.go:15-37`): `LoggerWithAuth` returns a logger enriched with `op`, `request_id`, `user_id`, `is_premium` — and writes a 401/500 itself if a context value is missing. This is a genuinely nice pattern: it collapses auth extraction and log enrichment into one call.

> ⚠️ **Authorization decisions read from JWT claims, not from the database.** `is_premium` and `is_verified` are snapshotted at token-issue time and trusted for the token's lifetime (15 min access TTL, but a client can refresh indefinitely). Consequences: a user whose premium is revoked (refund, chargeback, `SetPremiumStatus(userID, false)`) **keeps premium access until their access token expires**; email verification has the same lag. There is no `token_version` claim for invalidation on privilege change — `SetPremiumStatus` (`storage/postgres/payment.go:51-61`) does not touch existing tokens.

---

## 8. Data flows

### 8.1 Email reminder pipeline

```
POST /tasks or PATCH /tasks/{id}
        │
        ├── storage.SaveTask / UpdateTask   (PostgreSQL)
        └── go notifyScheduler()            ← fire-and-forget HTTP POST
                    │
                    ▼  POST http://email-notifier:8084  X-Webhook-Secret: ***
        notifiers/email/internal/webhook/handler.go
                    │
                    └── scheduler.Reschedule()  → non-blocking send on buffered chan (cap 1)
                                  │
        ┌─────────────────────────┴──────────────────────────────┐
        │ shared/scheduler: timer-driven poll loop                │
        │  poll(): GetPendingTasksWithUsers (single JOIN, no N+1) │
        │  → sender.Send (net/smtp, TLS)                          │
        │  → MarkTaskAsNotified                                    │
        │  next delay = time.Until(GetNearestPendingReminderAt())  │
        │  fallback = 5 min                                        │
        └──────────────────────────────────────────────────────────┘
```

The **coalescing reschedule channel** and the **timer drain** (`scheduler.go:40-45,59-68`) are well done: bursts of webhooks collapse into one poll instead of stampeding. Verification emails ride the *same* webhook via `payload.Type == "verification"`.

### 8.2 Activity logging (gRPC → MongoDB)

```
any mutating handler (tasks, categories, pomodoros, register, login)
        │
        └── activitylogger.Client.LogEvent(userID, action, entityID, details)
                    │  go func() { ... }              ← unbounded goroutine per event
                    ▼
        gRPC :50051  LogEvent  →  activity-logger  →  MongoDB todo_logs.activities
                                                        (index: none declared)
```

Read path: `GET /me/logs` → `GetLogs(ctx, userID, limit, offset)` → MongoDB `find` sorted by `created_at` desc, default limit 50. The gRPC handler **does** apply a 5s `context.WithTimeout` on both insert and find (`activity-logger/cmd/main.go:48,83`).

### 8.3 Payments (YooKassa)

```
POST /payments/create  → yookassa API → INSERT payments(status=pending) → return confirmation_url
POST /payments/sync    → find latest pending → query gateway → update + grant premium
POST /webhooks/yookassa → IP allowlist → re-verify via gateway API → update + grant premium
```

The webhook's **server-side re-verification** (`webhook.go:108-122`) — asking YooKassa what the payment status actually is instead of trusting the payload — is the correct pattern and better than most tutorials.

### 8.4 Gamification

`domain/stats.go` holds the whole ruleset in Go: `CalculateLevel` (100 XP for level 1, +50 per level, cap 100), `GetStreakMultiplier` (×2.0 at 7 days, ×1.5 at 5, ×1.2 at 3), 12 achievements in `AvailableAchievements`, plus daily-quest types. Storage side: `ApplyStatsDelta`, `UnlockAchievement` (idempotent via `UNIQUE(user_id, code)`), `CheckAndUnlockAchievements`, `GetGamificationProfile`. Reward constants: `TaskCompleteReward = 10`, `PomodoroRewardPoints = 20`, `PomodoroPenaltyPoints = -30`.

---

## 9. Test coverage

| Module | Files | Tests | Status |
|---|---|---|---|
| `backend` | 38 | ~180 | ✅ all green |
| `notifiers/shared` | 1 | 2 | ✅ green |
| `notifiers/email` | 2 | 8 | ✅ green |
| `activity-logger` | 1 | 1 | ⚠️ needs Docker (env-blocked here) |

**Strongest suites (by test count):** `storage/postgres/postgres_test.go` (49), `tasks/update` (10), `tasks/save` (9), `tasks/get` (8), `categories/{create,update}` (7 each), `tests/e2e/e2e_test.go` (6, testcontainers-based).

**Packages with code but no tests at all:**
`internal/http-server/router` · `internal/workers/cleanup` · `handlers/categories/getone` · `handlers/stats/update` · `internal/http-server/helpers` · `internal/lib/api/response` · `internal/lib/handlers` · `internal/lib/sl` · `internal/storage` (interface + sentinel errors) · `backend/utils` · `backend/cmd` · `notifiers/shared/storage{,/postgres}` · `notifiers/email/cmd` · `notifiers/email/internal/formatter` · `notifiers/email/internal/webhook`

> 🔴 **Three of these gaps sit on failure paths that matter:** the email webhook handler (secret validation, the detached send goroutine), the token-cleanup worker, and `router.go` itself (so nothing verifies that `/dev/toggle-premium` is truly absent in prod). The `notifiers/shared/storage/postgres` gap is notable because that package owns the scheduler's query.

---

## 10. CI/CD & operations

### `.github/workflows/ci.yml`
Three independent jobs on `push`/`pull_request` to `main`: `backend-ci` (Go 1.25.0, `go test -v ./...`), `notifiers-ci` (shared + email), `activity-logger-ci`. This **newly covers the previously untested modules** — a real improvement over the old single-module pipeline.

### `.github/workflows/cd.yml`
On `push` to `main`: build+push four images to GHCR (`-backend`, `-email`, `-activity-logger`, `-frontend`), then `deploy` job → ssh-agent → Ansible → `ansible/deploy.yml` with `image_base`, generating `.env` from `secrets.PROD_ENV_FILE`. `activity-logger` and `mongo` are deployed by `docker-compose.yml`, **not** by the CD workflow's image list — the logger image is pushed but its deployment depends entirely on the compose file on the server.

> 🔴 **CI and CD are independent triggers on the same event.** Both listen to `push: main`; `cd.yml` has no `needs:`/`workflow_run` link to `ci.yml` and no test step of its own. A red commit deploys to production in parallel with its own failing tests. This is the single most impactful pipeline defect.

### 🔴 Linter is dead weight
`backend/.golangci.yml` (47 lines) enables `errcheck`, `gosec`, `staticcheck`, `govet`, `bodyclose`, `nilerr`, `unused`, `gofmt`, `goimports`, `gosimple`, `stylecheck`. **No workflow invokes `golangci-lint`.** The config even contains a codebase-specific hint ("критично для вашего notifyScheduler!") that suggests it once ran.

### Other operational gaps

| # | Finding |
|---|---|
| 1 | **No observability whatsoever**: no `/metrics`, no Prometheus client, no expvar, no pprof, no tracing despite OTel being in the module graph. Only signal is structured `slog` + chi `RequestID`. |
| 2 | `docker-compose.yml` defines **no `healthcheck`** on any of the five services, so `depends_on` only orders container start, not readiness. `depends_on: [activity-logger]` on backend (line 29-31) gives a false sense of ordering. |
| 3 | **`backend/myprogram` is a 31.5 MB untracked build artifact** sitting in the repo tree. `.gitignore` covers `*.exe` and `tmp/` but not extensionless binaries. |
| 4 | Goose migrations run **inside** `backend/cmd/main.go:68-71` on every replica start — a race the moment there is more than one backend container. |
| 5 | No `Makefile`, `Taskfile`, `pre-commit` config, or `goreleaser`; the only task runner is `dev.ps1` (Windows-only PowerShell). |
| 6 | `.golangci.yml` sets `issues.exclude-use-default: false`; combined with `govet.enable-all` this would produce a large first-run report — budget for that if the linter is wired into CI. |
| 7 | CI pins **Go 1.25.0** while this machine runs **go1.27.1**, so local green ≠ CI green by construction. |

---

## 11. Verified defects

Every item below was confirmed by reading the code. Severity reflects blast radius, not fix effort.

### 🔴 Critical — money path

| # | Location | Defect | Consequence |
|---|---|---|---|
| **C1** | `payments/webhook/webhook.go:115-118` | The guard `verifiedPayment == nil \|\| verifiedPayment.Status != Succeeded` is correct, but the body logs `verifiedPayment.Status` — **a nil dereference on the very branch that handles nil**. | Guaranteed **panic** → 500 to the payment gateway → gateway retries. `webhook_test.go` has 4 tests; none covers `(nil, nil)`. |
| **C2** | `payments/sync/sync.go:100-110` | On `UpdatePaymentStatus` failure: log, then **grant premium anyway**, then return **200 `"succeeded"`**. | An error path reports success. Premium is granted with no corresponding payment record — the row was never updated. |
| **C3** | `webhook.go:124-137` + `sync.go:100-110` | Two writes (`UpdatePaymentStatus`, `GrantPremium`) with **no transaction**, and two independent callers race on them. | Double premium grant; partial state if the process dies between the writes. `GrantPremium`/`SetPremiumStatus` (`storage/postgres/payment.go:47-61`) are unconditional `UPDATE … WHERE id = $1`. |
| **C4** | `payments/create/create.go:50` | **Idempotency key is random per request.** | Double-click or client retry creates two gateway payments → **double charge**. The SDK supports the key (`:52` forwards it); the value is simply useless. |
| **C5** | `payments/create/create.go:82-85` | DB insert failure is **only logged**; the handler still returns 200 with the confirmation URL. | Orphaned gateway payment. The later webhook does `UPDATE … RETURNING user_id` against a missing row → `sql.ErrNoRows` → wrapped error → **every subsequent webhook retry 500s forever**. Compounds C2's `sync` path. |
| **C6** | `payments/webhook/webhook.go:49-56` | Forwarded headers (`X-Real-IP`, `X-Forwarded-For`) are trusted from any peer that `IsPrivate()`. Inside the Docker bridge network every hop is private, so the allowlist can be spoofed by any container that can reach the backend. | The IP allowlist is weaker than it appears. There is **no HMAC signature check** on the webhook body and no `MaxBytesReader` (`:97`). |

### 🟠 High — data loss, duplicates, resource exhaustion

| # | Location | Defect | Consequence |
|---|---|---|---|
| **H1** | `clients/activitylogger/activitylogger.go:36` | **One unbounded goroutine per logged event**, no semaphore/pool/queue, no retry; failure is only logged (`:59-66`). | Traffic spike → goroutine avalanche; audit events silently vanish. No counter tracks the loss. |
| **H2** | `notifiers/shared/scheduler/scheduler.go:122-124` | `MarkTaskAsNotified` failure is logged, loop continues. | **Duplicate emails** on the next poll — the exact failure mode a notifier must not have. |
| **H3** | `notifiers/shared/scheduler/scheduler.go:26-38` | The `intervals []time.Duration` constructor parameter is **completely unused**; `fallbackPollInterval` is a hardcoded 5 min (`:24`). | A configured feature does nothing. `cfg.Intervals` is loaded (`config.go:15`), passed in (`email/cmd/main.go:49-54`), and dropped. |
| **H4** | `notifiers/shared/scheduler/scheduler.go:117` | `sender.Send(user, task, time.Duration(0))` — always zero. | Interval data never reaches the formatter; `NotificationEvent.Interval` is dead. |
| **H5** | `scheduler.go:89-92` | When the nearest reminder is already past, delay is `0` → immediate re-poll. If marking keeps failing (H2), a due reminder produces a **tight loop of duplicate emails**. | Email bombing of a user by a stale row. |
| **H6** | `notifiers/email/internal/webhook/handler.go:67-72` | Verification email is sent in a **fresh goroutine after a 200** is written. | The response says "accepted"; failure is unobservable by the caller and unrecoverable. Same unbounded-goroutine shape as H1. |
| **H7** | `backend/cmd/main.go:53-56` | `activitylogger.New` error is **logged only**; `loggerClient` stays `nil` and is passed into the router (`router.go:59`) and on to handlers. | A nil client reaches handlers at runtime. Contrast `payments/create/create.go:39-44`, which correctly returns 503 for a nil YooKassa client — the codebase knows the pattern but did not apply it here. |
| **H8** | `clients/activitylogger/activitylogger.go:23` | The gRPC `ClientConn` is **never closed** — `Client` has no `Close()`. | Connection/fd leak; the `ctx` accepted by `New` (`:22`) is unused. |

### 🟡 Medium — correctness, security, robustness

| # | Location | Defect |
|---|---|---|
| **M1** | `notifiers/email/internal/webhook/handler.go:49` | Secret compared with `!=` instead of `crypto/subtle.ConstantTimeCompare` → timing oracle. (Good news: it **fails closed** with 503 when the secret is empty, `:44-48`.) |
| **M2** | `backend/cmd/main.go:97-102`, `notifiers/email/cmd/main.go:57-60` | `http.Server` has **no `WriteTimeout` / `ReadHeaderTimeout`**. Only `ReadTimeout` + `IdleTimeout` are set on the backend. Slowloris-friendly. |
| **M3** | `internal/workers/cleanup/tokencleanup.go:27,31` | The long-lived app `ctx` is passed to both DELETEs — **no per-query timeout**. One hung query blocks the single worker goroutine forever, and there is no `recover`. Interval hardcoded `24*time.Hour` at the call site (`cmd/main.go:58`). |
| **M4** | `notifiers/email/cmd/main.go:81` | `srv.Shutdown(context.Background())` — **unbounded**; a stuck connection prevents exit. |
| **M5** | `notifiers/email/cmd/main.go:73` | `log.Fatalf` **inside a goroutine** → `os.Exit` skips all defers (storage close, scheduler stop). |
| **M6** | `backend/cmd/main.go:46,64,70,108,123` | Every `os.Exit(1)` bypasses `defer storage.Close()` (`:48`) and `defer cancel()` (`:51`). |
| **M7** | `domain/payment.go:12` | Field named **`D`** with tag `json:"id"` — a typo for `ID`. Compiles, serializes correctly, reads as a mistake. |
| **M8** | `notifiers/email/internal/webhook/handler.go:76-88` | Any unknown `payload.Type` logs and **triggers a reschedule**, returning 200. Typos in the producer are undetectable. |
| **M9** | `config/config.go:45` vs `:14` | `ClientURL` is declared **twice** — on `Config` and on the embedded `HTTPServer`. Two YAML keys with the same name; which wins depends on nesting, and the one the YooKassa handler receives is the outer `Config.ClientURL` (`router.go:161`). |
| **M10** | `httprate.LimitByIP(100, 1*time.Minute)` (`router.go:81`) | Rate limit is **per IP**, not per user/account. All users behind one NAT/CGNAT share 100 req/min, while `/login` and `/register` get no tighter limit — so credential stuffing is bounded only by IP rotation. |

### 🔵 Low — hygiene

| # | Location | Defect |
|---|---|---|
| **L1** | `backend/myprogram` | 31.5 MB untracked binary in the tree; `.gitignore` misses extensionless builds. |
| **L2** | `config/local.yaml:21-36` | Dead `cors:` YAML block (see §4). |
| **L3** | `config/config.go:36` | `retries_count` parsed, never used (see §4). |
| **L4** | `backend/.env.local.example`, `notifiers/email/.env.local.example` | Tracked but **empty**; README points at a non-existent `notifiers/email/deploy/`. |
| **L5** | `git status` | **48 files uncommitted** (`+3611/−877`) on `feature/backend`, including production code. Everything in this audit describes a working tree that exists on exactly one machine. |
| **L6** | `go.work` | Lists 5 modules but omits the root `.` module even though `pkg/` is imported everywhere via a `replace` in `activity-logger/go.mod:47`. |
| **L7** | `notifiers` | Uses `lib/pq` while the backend uses `pgx/v5` — two PostgreSQL drivers, two error-handling styles, in one repo. |
| **L8** | `notifiers/telegram-bot/` | Frozen skeleton with `BOT_TOKEN` config and a `/hello` handler; `users.telegram_id` exists and is never written. Either finish it or delete it — half-built features cost credibility. |
| **L9** | Migrations | Numbers `00002` and `00009` absent (15 files present). |
| **L10** | `frontend-churka/` | Full React 19 + MobX + TanStack + antd client (1,207 lines TS/TSX) that no workflow builds and no compose file runs. Two clients, one deployed — pick one and say why. |

---

## 12. Previous known-issues list — disposition

Carried over from the last revision, with a verdict on each:

| # | Old claim | Status now |
|---|---|---|
| 1 | `refresh/rerfresh.go` filename typo | ✅ **fixed** — directory is `handlers/auth/refresh/` |
| 2 | `GetUserByID` queried `WHERE user_id` on a column named `id` | ✅ **fixed** — `postgres.go:546`; `RotateRefreshToken` (`:474`) also replaced the old delete+insert pair with a single call |
| 3 | `refresh_tokens.user_id` BIGINT vs `users.id` INTEGER | ❌ **still present**, and now **four** tables have the mismatch (§5) |
| 4 | `UpdateTask` never touched `updated_at` | 🟡 **still present** — `updated_at` is absent from the dynamic `SET` list in `postgres.go:286`; the column exists (`00003`) and nothing else maintains it on update |
| 5 | No pagination on `GET /tasks` | ✅ **fixed** — limit/offset + total, plus status/search/category/sort filters (`get.go:94-151`) |
| 6 | Missing `return` after 401 in refresh → fall-through to 500 | ✅ **fixed** — `RotateRefreshToken` returns a single error; `refresh_test.go` has 3 tests |
| 7 | No graceful shutdown in `main.go` | ✅ **fixed** for the backend (`signal.NotifyContext` + timed `Shutdown`, `cmd/main.go:112-124`); 🟡 **still open** for email notifier (M4/M5) |
| 8 | `goose.Up` failure only warned | ✅ **fixed** — now `os.Exit(1)` (`:70`). ⚠️ But it still runs on every replica (ops gap #4) |
| 9 | No rate limiting | ✅ **fixed** — `httprate` 100/min by IP. 🟡 per-IP only (M10) |
| 10 | No `http.MaxBytesReader` on request bodies | ❌ **still present** — no body-size limit anywhere, including the payment webhook (C6) |

**Net:** 7 of 10 resolved — genuine, verifiable progress. The three survivors (FK types, `updated_at`, body limits) plus the new criticals from §11 are the honest outstanding list.

---

## 13. What this codebase does well

Worth stating explicitly, because the defects above are easier to write than the strengths:

1. **Handler-per-package layout** with narrow consumer-side interfaces (`TaskGetter`, `PaymentUpdater`, `PaymentFinder`, `PremiumToggler`) — this is why 41 test files exist and why they each run in ~1.5 s without a database.
2. **`LoggerWithAuth`** (`helpers/logger.go:15`) — one call for auth extraction + log enrichment + auth-failure response, applied consistently across ~25 handlers.
3. **Sentinel errors** in `internal/storage/storage.go` (12 of them) mapped to HTTP codes with `errors.Is`, instead of string matching.
4. **`RotateRefreshToken`** is a single atomic storage operation rather than the delete-then-insert race the old version had.
5. **YooKassa webhook re-verifies server-side** (`webhook.go:108-122`) instead of trusting the payload — the correct pattern.
6. **Scheduler coalescing** via buffered channel + timer drain (`scheduler.go:40-45,59-68`) — burst-safe, and `GetPendingTasksWithUsers` is a single JOIN with no N+1.
7. **`UnlockAchievement` is idempotent** by `UNIQUE(user_id, code)` — no double-reward race.
8. **`RETURNING id` everywhere** — no `lastInsertId` guessing, no read-after-write race.
9. **Embedded migrations** (`embed.FS` + goose) — schema travels with the binary, and seven rounds of schema evolution are visible as 15 discrete migrations.
10. **`middleware/logger`** routes log levels by status class and suppresses health-check noise — small, but it shows operational empathy.
11. **`http.Server` timeouts, `RequestID`, `Recoverer`, CORS, and rate limiting are all wired** — the middleware stack is more complete than most portfolio projects.

---

## 14. Suggested reading order for a new contributor

1. `README.md` → `docker-compose.yml` → `dev.ps1` (what runs where) — *note the two broken setup references in §4 first*
2. `backend/cmd/main.go` (154 lines: the whole startup story)
3. `backend/internal/http-server/router/router.go` (172 lines: **the authoritative API map**)
4. `backend/internal/domain/` + `backend/migrations/` (the data model)
5. One vertical slice end-to-end: `handlers/tasks/save` → `storage/postgres.SaveTask` → webhook → `notifiers/shared/scheduler`
6. `backend/internal/lib/jwt` + `middleware/auth` (how identity flows)
7. `activity-logger/cmd/main.go` + `proto/activity_logger/v1/activity_logger.proto` (the second service)

---

## 15. Priority backlog

Ordered by (blast radius × verifiability). `docs/ROADMAP-ADVANCED.md` holds the feature roadmap; this section covers only what the audit itself found.

| Priority | Items | Why first |
|---|---|---|
| **P0 — correctness on the money path** | C1, C2, C3, C4, C5, C6 | A nil-dereference panic, an error path returning success, non-transactional grants, and a useless idempotency key — all in code that handles payments. Each fix is small; together they are the strongest reliability signal available. Regression tests: `(nil,nil)` finder, duplicate webhook replay, `sync`+`webhook` race, DB failure mid-transaction, under `-race`. |
| **P1 — stop losing data and duplicating mail** | H1, H2, H5, H6, H7, H8 | Bounded worker pool for activity events; make `MarkTaskAsNotified` failure terminal for that delivery attempt; fix the zero-delay tight loop; nil client → 503 instead of a nil-map access; close the gRPC conn. |
| **P2 — make it measurable** | §10 #1 | Nothing above can be *proven* fixed without metrics. RED metrics + `pgxpool.Stat()` + pprof + `/health/ready` + a k6 baseline. Turns every later claim into a number. |
| **P3 — cheap, high-signal gaps** | linter in CI, CI→CD dependency, `MaxBytesReader` (C6), server timeouts (M2), `subtle.ConstantTimeCompare` (M1), `backend/myprogram` (L1), empty `.env.local.example` + README path (L4), dead `cors:` block (L2) | Most are minutes of work and each removes an obvious reviewer objection. |
| **P4 — schema and migration hygiene** | missing scheduler index (§5), FK type unification, migration-numbering note, `updated_at` in `UpdateTask` (old #4), CHECK constraints on `user_stats` | Index first (the H-series queries depend on it); the rest is coherent-schema work. |
| **P5 — documentation truth** | OpenAPI drift (12 operations), reconcile `README.md` / `CODEBASE.md` / `router.go`, note the `00002`/`00009` gaps | Three sources of truth currently disagree; only `router.go` is right. |
| **P6 — decide, don't accumulate** | `frontend-churka` vs `frontend` (L10), `telegram-bot` (L8), `lib/pq` vs pgx (L7), unused `retries_count` (L3) | Each is either finished or deleted. Half-built surfaces read as indecision. |

---

## 16. Uncommitted-work warning

This audit describes `feature/backend` plus 48 uncommitted files. `git log -1` is `5836d61 "merge with frontend branch"`, so a reviewer cloning the repository **today** sees none of it: no payments, no gamification, no activity-logger, no `frontend-churka`, no bulk operations, and only 2 of the 15 migrations. Before this project is shown to anyone, that working tree needs to be committed and pushed — and the two `.env` files holding real local secrets must stay untracked (they currently are: `.env` is in `.gitignore`, and `git ls-files` shows only the three example files).
