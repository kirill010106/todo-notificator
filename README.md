# ToDo Notificator

Monorepo-проект для управления задачами с email-уведомлениями, Pomodoro-таймером и JWT-аутентификацией.

## Что в репозитории сейчас

- Backend API на Go (chi + pgx + goose): задачи, категории, статистика, pomodoro, auth.
- Веб-клиент на Alpine.js в папке frontend.
- Дополнительный React-клиент (Vite + React + TS) в папке frontend-react.
- Email notifier (Go) с webhook-триггером и периодическим polling.
- Telegram bot присутствует как заготовка, но не интегрирован в основной flow.
- OpenAPI/Swagger документация синхронизирована с backend.

## Структура

```text
backend/           REST API (основной сервис)
frontend/          Основной статический веб-клиент (Alpine.js)
frontend-react/    Альтернативный React-клиент
notifiers/email/   Email-нотификатор
notifiers/shared/  Общие доменные модели/хранилище для нотификаторов
docs/              Swagger UI + openapi.yaml
```

## Текущий API

Базовый префикс: /api/v1

### Public endpoints

- POST /register
- POST /login
- POST /refresh
- GET /verify
- GET /health

### Protected endpoints

- POST /logout
- POST /verify/resend
- GET /tasks
- POST /tasks
- PATCH /tasks/{task_id}
- DELETE /tasks/{task_id}
- GET /categories
- GET /categories/{category_id}
- POST /categories
- PATCH /categories/{category_id}
- DELETE /categories/{category_id}
- GET /me/stats
- PATCH /me/stats
- GET /me/logs
- POST /pomodoros/start
- POST /pomodoros/{id}/pause
- POST /pomodoros/{id}/stop

Полная спецификация: docs/openapi.yaml

## Требования

- Go 1.25+
- Node.js 20+
- PostgreSQL 14+
- (опционально) Docker для e2e и/или локальной БД
- (опционально) air для hot reload

## Быстрый старт (локальная разработка)

### 1. Клонирование

```bash
git clone https://github.com/kirill010106/todo-notificator.git
cd toDoNotificator
```

### 2. Подготовка БД

Вариант с локальным PostgreSQL: создайте БД и пользователя вручную.

Вариант с Docker:

```bash
docker run --name todo-pg \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=todo \
  -p 5432:5432 \
  -d postgres:16-alpine
```

### 3. Настройка backend env

Скопируйте шаблон в backend/.env (backend читает .env из своей рабочей директории):

```powershell
Copy-Item .env.example backend/.env
```

Проверьте backend/.env:

```dotenv
DATABASE_URL=postgres://postgres:postgres@localhost:5432/todo?sslmode=disable
CONFIG_PATH=./config/local.yaml
APP_SECRET=replace-with-random-secret
```

Важно: backend/internal/config/config.go требует CONFIG_PATH и APP_SECRET.

### 4. Настройка email notifier env

Создайте notifiers/email/.env на базе шаблона:

```powershell
Copy-Item notifiers/email/deploy/.env.notifier.example notifiers/email/.env
```

Минимально заполните:

```dotenv
DATABASE_URL=postgres://postgres:postgres@localhost:5432/todo?sslmode=disable
SMTP_USERNAME=your-email@example.com
SMTP_PASSWORD=your-password
WEBHOOK_SECRET=same-secret-as-backend-webhook
```

Примечание: WEBHOOK_SECRET обязателен для email-нотификатора.

### 5. Запуск сервисов

#### Вариант A: через dev.ps1 (backend + email notifier)

```powershell
./dev.ps1
```

Скрипт использует air в обоих сервисах. Если air не установлен:

```bash
go install github.com/air-verse/air@latest
```

#### Вариант B: вручную в двух терминалах

Backend:

```bash
cd backend
go run ./cmd/main.go
```

Email notifier:

```bash
cd notifiers/email
go run ./cmd/main.go
```

Миграции backend выполняет автоматически при старте.

### 6. Запуск фронтенда

#### Основной Alpine.js клиент

```bash
npx --yes http-server . -p 3030
```

Откройте: http://localhost:3030/frontend/

#### React-клиент (альтернативный)

```bash
cd frontend-react
npm install
npm run dev
```

По умолчанию React-клиент обращается к http://localhost:8082/api/v1 (см. frontend-react/src/api/client.ts).

## Swagger UI

```bash
cd docs
npm install
node server.js
```

UI будет доступен на http://localhost:8080

## Тестирование

### Backend unit/integration

```bash
cd backend
go test ./... -count=1
```

### E2E (build tag e2e)

```bash
cd backend
go test -tags=e2e ./tests/e2e -count=1 -v
```

E2E используют testcontainers. Если Docker недоступен в окружении, можно передать внешний DSN:

```powershell
$env:E2E_POSTGRES_DSN = "postgres://postgres:postgres@localhost:5432/todo_e2e?sslmode=disable"
go test -tags=e2e ./tests/e2e -count=1 -v
```

## Лицензия

MIT, см. LICENSE.
