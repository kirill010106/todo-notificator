# Architecture

## Modules (go.work)

```
toDoNotificator/                     ← workspace root
├── go.work
│
├── backend/                         ← module: github.com/kirill010106/todo-notificator
├── notifiers/                       ← module: github.com/kirill010106/todo-notificator/notifiers
│   └── email/                       ← module: github.com/kirill010106/todo-notificator/notifiers/email
│
└── frontend/
    └── index.html                   ← SPA (Alpine.js + Tailwind CDN)
```

---

## Backend

```
backend/
├── cmd/main.go                      ← entry point, router, graceful shutdown
├── config/local.yaml
├── migrations/
│   ├── 00001_init_db.sql
│   └── 00002_update_tables.sql
├── migrations.go                    ← embed.FS + goose
└── internal/
    ├── config/config.go             ← cleanenv + godotenv
    ├── domain/
    │   ├── task.go
    │   ├── user.go
    │   └── token.go                 ← RefreshToken
    ├── storage/
    │   ├── storage.go               ← Storage interface
    │   └── postgres/postgres.go     ← pgx/v5/stdlib реализация
    ├── lib/
    │   ├── jwt/jwt.go               ← генерация/парсинг access токена
    │   ├── sl/sl.go                 ← slog helpers
    │   ├── api/response/response.go ← стандартные JSON ответы
    │   └── handlers/slogpretty.go
    └── http-server/
        ├── middleware/auth/auth.go  ← JWT middleware, GetUserID(ctx)
        ├── handlers/
        │   ├── health/health.go
        │   └── auth/
        │       ├── register/register.go
        │       ├── login/login.go
        │       ├── refresh/refresh.go    ← ротация refresh токена
        │       └── logout/logout.go
        └── tasks/
            ├── save/save.go         ← POST   /api/v1/tasks
            ├── get/get.go           ← GET    /api/v1/tasks
            ├── update/update.go     ← PATCH  /api/v1/tasks/{id}
            └── delete/delete.go     ← DELETE /api/v1/tasks/{id}
```

### API routes

```
POST   /api/v1/auth/register
POST   /api/v1/auth/login
POST   /api/v1/auth/refresh
POST   /api/v1/auth/logout

GET    /api/v1/tasks           ← ?page=1&limit=20
POST   /api/v1/tasks
PATCH  /api/v1/tasks/{id}
DELETE /api/v1/tasks/{id}

GET    /health
```

---

## Notifiers

```
notifiers/
├── go.mod
├── shared/                          ← переиспользуется всеми нотификаторами
│   ├── domain/domain.go             ← Task, User, NotificationEvent
│   ├── storage/
│   │   ├── storage.go               ← Storage interface
│   │   └── postgres/postgres.go     ← реализация (только SELECT)
│   └── scheduler/scheduler.go       ← планировщик уведомлений (time.AfterFunc)
│
├── email/                           ← email нотификатор
│   ├── go.mod
│   ├── config/config.yaml
│   └── cmd/main.go                  ← entry point
│       └── internal/
│           ├── config/config.go     ← SMTP + DB + intervals + webhook
│           ├── formatter/
│           │   ├── formatter.go     ← html/template
│           │   └── templates/deadline.html
│           ├── sender/sender.go     ← net/smtp (TLS/STARTTLS)
│           └── webhook/             ← 🔧 В РАБОТЕ
│               └── handler.go       ← HTTP POST /webhook/task-created
│
└── telegram-bot/                    ← заморожен
    ├── cmd/main.go
    └── internal/config/config.go
```

---

## Webhook (в работе)

**Цель:** backend сигнализирует email нотификатору о новых задачах мгновенно.

**Статус:** реализуем сейчас.

**Осталось сделать:**

```
[ ] notifiers/email/internal/webhook/handler.go   ← HTTP обработчик
[ ] notifiers/email/internal/config/config.go     ← добавить Webhook struct
[ ] notifiers/email/config/config.yaml            ← webhook.address + webhook.secret
[ ] notifiers/email/cmd/main.go                   ← запуск HTTP сервера
[ ] backend/internal/http-server/tasks/save/save.go  ← fire-and-forget POST
[ ] backend/config/local.yaml                     ← webhook_url + webhook_secret
[ ] .env                                          ← WEBHOOK_SECRET=...
```

**Схема:**

```
POST /api/v1/tasks
    │
    ├─ storage.SaveTask()
    └─ go notifyScheduler()          ← горутина, не блокирует ответ
                │
                └─ POST :8084/webhook/task-created
                   X-Webhook-Secret: ***
                            │
                            └─ scheduler.Reschedule()
```

---

## Зависимости между модулями

```
                    ┌─────────────────────────┐
                    │  notifiers/shared        │
                    │  (domain, storage,       │
                    │   scheduler)             │
                    └────────────┬────────────┘
                                 │ импортирует
                    ┌────────────▼────────────┐
                    │  notifiers/email         │
                    │  (formatter, sender,     │
                    │   webhook)               │
                    └─────────────────────────┘

backend и notifiers НЕ зависят друг от друга.
Связь только через PostgreSQL и HTTP webhook.
```

---

## Tech Stack

| Слой         | Технология                    |
|--------------|-------------------------------|
| HTTP         | chi v5                        |
| Auth         | golang-jwt/v5 (HS256)         |
| DB           | PostgreSQL, pgx/v5            |
| Migrations   | pressly/goose/v3              |
| Config       | cleanenv + godotenv           |
| Email        | net/smtp (stdlib)             |
| Frontend     | Alpine.js 3 + Tailwind CSS v3 |
| Scheduler    | time.AfterFunc                |
