# ToDo Notificator

Монорепозиторий для управления задачами с email-напоминаниями, Pomodoro, JWT-аутентификацией и геймификацией.

## Возможности

- Задачи и категории: создание, изменение, удаление, массовое завершение и удаление задач.
- Фильтрация задач по статусу и категории, поиск по заголовку/описанию, сортировка и пагинация.
- Pomodoro с привязкой к задаче или свободной сессией, перерывами и завершением/отменой.
- Профиль, XP, уровни, серии активности, достижения и ежедневные задания.
- Подтверждение email, повторная отправка письма, access/refresh tokens и выход со всех устройств.
- Email-напоминания через SMTP: webhook пересчитывает расписание, polling служит резервным механизмом.
- История активности через отдельный gRPC-сервис с MongoDB.
- Премиум через YooKassa: создание платежа, webhook и проверка последнего ожидающего платежа.
- Метрики Prometheus и конфигурация Grafana.

В бесплатном тарифе доступны до 3 категорий, до 3 активных напоминаний и последние 5 записей истории. Профиль возвращает лимиты; значение `-1` означает отсутствие лимита у Премиум. Некоторые ограничения проверяются по признакам в access token, поэтому после подтверждения email или изменения тарифа обновите токены через `/refresh` либо войдите заново.

## Структура

```text
backend/           REST API: Go, chi, pgx, goose; SQL-миграции и тесты
frontend/          React + TypeScript + Vite: задачи, фокус, прогресс и награды
activity-logger/   gRPC-сервис истории активности с MongoDB
notifiers/email/   SMTP-уведомления и прием webhook от backend
notifiers/shared/  Модели, PostgreSQL-хранилище и планировщик уведомлений
notifiers/telegram-bot/  Заготовка Telegram-бота, не входит в dev.ps1 и Compose
proto/             Контракт gRPC activity-logger
pkg/               Сгенерированный Go-код gRPC
scripts/loadtest/  Нагрузочный сценарий пользователей и задач
docs/             Swagger UI и OpenAPI (openapi.yaml)
deploy/            Настройки Grafana
ansible/           Развертывание
```

Go-модули объединены в `go.work`. React-клиент находится в `frontend/`.

## API

Базовый префикс: `/api/v1`. Полный контракт с телами запросов, ответами и ошибками: [docs/openapi.yaml](docs/openapi.yaml). Источником маршрутов служит `backend/internal/http-server/router/router.go`.

| Доступ | Методы и пути относительно `/api/v1` |
| --- | --- |
| Публичный | `POST /register`, `POST /login`, `POST /refresh`, `GET /verify?token=...`, `GET /health`, `GET /` |
| Webhook YooKassa, без JWT, с проверкой IP | `POST /webhooks/yookassa` |
| Авторизация, Bearer JWT | `POST /logout`, `POST /verify/resend` |
| Начальное состояние и профиль, Bearer JWT | `GET /me/bootstrap`, `GET /me/profile`, `PATCH /me/profile`, `GET /me/achievements`, `GET /me/quests` |
| План дня, фокус и награды, Bearer JWT | `GET /me/today`, `GET /me/focus`, `POST /me/focus/resume`, `POST /me/focus/rest/finish`, `POST /me/quests/{code}/claim`, `POST /me/rewards/daily-chest/claim` |
| Статистика и история, Bearer JWT | `GET /me/stats`, `GET /me/logs` |
| Задачи, Bearer JWT | `GET /tasks`, `POST /tasks`, `PATCH /tasks/{task_id}`, `DELETE /tasks/{task_id}`, `POST /tasks/bulk-complete`, `POST /tasks/bulk-delete` |
| Категории, Bearer JWT | `GET /categories`, `POST /categories`, `GET /categories/{category_id}`, `PATCH /categories/{category_id}`, `DELETE /categories/{category_id}` |
| Pomodoro, Bearer JWT | `GET /pomodoros/active`, `POST /pomodoros/start`, `POST /pomodoros/{id}/pause`, `POST /pomodoros/{id}/stop` |
| Платежи, Bearer JWT | `POST /payments/create`, `POST /payments/sync` |
| Только `ENV=local`, Bearer JWT | `POST /dev/toggle-premium` |

Вне префикса API доступны `GET /` (информация о сервисе) и `GET /ping` (ответ `pong` без проверки БД). `GET /api/v1/health` проверяет доступность PostgreSQL. Метрики находятся на отдельном HTTP-сервере: локально `http://localhost:9091/metrics`.

Для защищенных методов передавайте `Authorization: Bearer <access_token>`. В локальном конфиге access token действует 15 минут, refresh token — 168 часов. Актуальные Premium и подтверждение email проверяются по БД. Общий лимит HTTP-сервера: 100 запросов в минуту на IP; превышение возвращает JSON `429` с `Retry-After`.

`GET /tasks` принимает `limit` (по умолчанию 20, значения выше 100 ограничиваются до 100), `offset` (по умолчанию 0), `status` (`all`, `pending`, `done`, `burnt`), `search`, `category_id`, `sort_by` (`created_at`, `deadline`) и `order` (`asc`, `desc`). По умолчанию задачи сортируются по дате создания убывающе; при `sort_by=deadline` — по дедлайну возрастающе, без даты в конце. Для изменения направления передавайте и `sort_by`, и `order`.

`GET /me/bootstrap` возвращает пользователя с лимитами и `timezone`, категории, первые 20 задач, статистику, геймификацию и `focus` с серверным временем и сохраненными перерывами. Поле `active_pomodoro` сохранено для совместимости. При ошибке загрузки фокуса сервер возвращает ошибку, а не состояние ожидания.

Ошибки содержат `code`, `message`, `request_id`, `field_errors`; прежнее поле `error` сохранено для совместимости. Webhook YooKassa при успехе возвращает пустой `200`, а ряд ошибок — `text/plain`.

`PATCH /tasks/{task_id}` принимает явный `null` для снятия срока, напоминания и категории; отсутствующие поля сохраняются. `GET /tasks` также принимает `deadline_from` (включительно), `deadline_before` (исключительно), `has_deadline`. `GET /me/today` возвращает отдельные страницы и полные счетчики групп: просроченные до серверного текущего времени и оставшиеся сроки до конца дня в часовом поясе профиля. Изменить пояс можно через `PATCH /me/profile` с `timezone` IANA, по умолчанию UTC.

Создание/редактирование задач, массовое завершение, команды Pomodoro и получение дневных наград поддерживают `Idempotency-Key` (до 128 символов). Ключ хранится в пределах аккаунта вместе с успешным ответом в одной транзакции. Повтор того же запроса возвращает прежний результат; другое действие с тем же ключом — `409`. Ответы этих команд дополнены `operation_id`, `result`, `rewards`, `stats`, `unlocked_achievements` и `focus`. Сохраняйте ключ при повторе после потери ответа. Подробная схема — в Swagger.

## Статистика, награды и удаление

Статистику рассчитывает сервер. Публичного `PATCH /me/stats` нет: такой запрос получает `405`. Результат действия содержит обновленные значения; отдельно доступны `GET /me/stats` и `GET /me/bootstrap`.

- Первое завершение задачи дает 10 очков. Повторное открытие и завершение не дублируют награду.
- Засчитанный Pomodoro дает 20 очков; если одновременно впервые завершена задача — еще 10. Достижения могут добавить бонусы.
- Порог засчитывания сессии — 24 минуты по времени БД для таймера на 25 минут. Короткая сессия не дает награды за Pomodoro; явное завершение задачи все равно может дать ее обычные 10 очков.
- Массовое завершение использует те же правила. Изменения задач, награды и достижения сохраняются в одной транзакции; ошибки записи откатывают всю операцию.
- Повторная остановка завершенной сессии без прежнего ключа возвращает `404`; с тем же ключом — сохраненный успех. Запуск с чужой или несуществующей задачей возвращает `404`.
- Бонусы достижений и дневных заданий не продлевают стрик. Получение награды задания и дневного сундука защищено уникальностью аккаунт/день/код. Размер XP задает сервер; сундук дает 50 XP за выполнение всех заданий. Прогресс сохраняется при удалении задачи; повторное сохранение выполненной задачи не добавляет завершений.
- Удаление категории сохраняет задачи без категории. Удаление задачи завершает активный таймер без штрафа и сохраняет историю сессий без ссылки на задачу. Массовое удаление действует аналогично.

При ручном прерывании сессии с задачей сохраняется штраф 30 очков; свободная сессия прерывается без штрафа и остается в истории. Уровень пересчитывается из очков. Подробные правила и транзакционные гарантии: [docs/BUSINESS-LOGIC.md](docs/BUSINESS-LOGIC.md).

Backend автоматически применяет миграции при запуске. `00018` задает правила удаления, `00019` добавляет состояние фокуса, часовой пояс, идемпотентные операции и дневные награды, `00020` — журнал дневных действий над задачами. Перед обновлением сохраните резервную копию БД; новый клиент требует обновленного backend.

## Требования

- Go 1.26+ для работы с текущим `go.work` и корневым модулем.
- Node.js 24 и npm для сборки фронтенда и Swagger UI.
- PostgreSQL (в локальном скрипте используется `16-alpine`).
- MongoDB для истории активности (в локальном скрипте используется MongoDB 7).
- Docker Desktop и PowerShell для `dev.ps1`; Air для перезапуска сервисов при изменениях.

## Локальный запуск

Все команды ниже выполняются из корня репозитория, если не указано иначе.

### 1. Настройки backend и email

Скопируйте существующий шаблон в рабочие директории сервисов:

```powershell
Copy-Item .env.example backend/.env
Copy-Item .env.example notifiers/email/.env
```

Для backend укажите:

```dotenv
DATABASE_URL=postgres://postgres:postgres@127.0.0.1:5433/todo?sslmode=disable
CONFIG_PATH=./config/local.yaml
APP_SECRET=replace-with-a-long-random-secret
WEBHOOK_URL=http://localhost:8084
WEBHOOK_SECRET=replace-with-shared-webhook-secret
CLIENT_URL=http://localhost:3030
ACTIVITY_LOGGER_ADDR=localhost:50051
```

Для email-сервиса укажите тот же DSN и тот же `WEBHOOK_SECRET`, а также действующие SMTP-настройки:

```dotenv
DATABASE_URL=postgres://postgres:postgres@127.0.0.1:5433/todo?sslmode=disable
EMAIL_CONFIG_PATH=./config/local.yaml
WEBHOOK_SECRET=replace-with-shared-webhook-secret
SMTP_HOST=smtp-relay.brevo.com
SMTP_PORT=587
SMTP_USERNAME=your-smtp-login
SMTP_PASSWORD=your-smtp-key
SMTP_FROM=your-verified-sender@example.com
SMTP_SENDER_NAME=ToDoNotificator
APP_URL=http://localhost:3030
```

`APP_SECRET` и `CONFIG_PATH` обязательны для backend. Для email обязательны `DATABASE_URL`, `WEBHOOK_SECRET`, `SMTP_USERNAME` и `SMTP_PASSWORD`; адрес отправителя должен соответствовать настройкам SMTP-провайдера. Backend читает `.env` из рабочей директории; email также ищет `.env` в нескольких родительских директориях. Уже заданные переменные окружения имеют приоритет над `.env`.

Локальные YAML-конфиги находятся в `backend/config/local.yaml` и `notifiers/email/config/local.yaml`. Backend слушает `8082`, email webhook — `8084`, activity-logger — `50051`, метрики — `9091`.

### 2. Запуск через dev.ps1

Установите Air, если его нет в `PATH`:

```powershell
go install github.com/air-verse/air@latest
```

Запустите Docker Desktop, затем:

```powershell
./dev.ps1
```

Скрипт создает или запускает PostgreSQL (`todo-local-pg-alt`, порт `5433`) и MongoDB (`todo-mongodb`, порт `27017`), ждет готовности и открывает три PowerShell-окна с Air: backend, email и activity-logger. DSN PostgreSQL и пути конфигов он передает сам; секреты и SMTP берутся из подготовленных `.env`. Скрипт завершает ранее запущенные процессы `app` этого проекта. При остановке сервисов контейнеры БД остаются запущенными.

### 3. Альтернатива: ручной запуск

Для тех же локальных портов создайте базы:

```powershell
docker run --name todo-local-pg-alt -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=todo -p 5433:5432 -d postgres:16-alpine
docker run --name todo-mongodb -p 27017:27017 -d mongo:7
```

Если эти контейнеры уже созданы, используйте `docker start todo-local-pg-alt todo-mongodb`.

Запустите каждый сервис в отдельном терминале из корня проекта:

```powershell
# Терминал 1
Set-Location activity-logger
$env:MONGO_URL = 'mongodb://127.0.0.1:27017'
go run ./cmd/main.go
```

```powershell
# Терминал 2
Set-Location backend
go run ./cmd/main.go
```

```powershell
# Терминал 3
Set-Location notifiers/email
go run ./cmd/main.go
```

Backend автоматически применяет встроенные SQL-миграции при старте. Перед работой email с задачами дождитесь завершения миграций. Регистрация и повторная отправка подтверждения требуют работающего email webhook. Если он недоступен, регистрация может вернуть `502` уже после создания пользователя; после запуска email войдите и используйте `/verify/resend`. Недоступный activity-logger не останавливает основной API; `/me/logs` возвращает `503`, если клиент не инициализирован, либо `500` при ошибке обращения к сервису.

### 4. Веб-клиент

Из корня проекта:

```powershell
Set-Location frontend
npm ci
npm run dev
```

Откройте [http://127.0.0.1:3030](http://127.0.0.1:3030). Vite перенаправляет `/api` на `127.0.0.1:8082`. Ссылки из писем и оплаты используют `APP_URL` и `CLIENT_URL`; используйте одно имя хоста для сохранения браузерной сессии. При необходимости задайте полный адрес API с `/api/v1` в `frontend/.env.local`: `VITE_API_BASE_URL=http://localhost:8082/api/v1`.

Для production выполните `npm run build`; результат — `frontend/dist/`. Dockerfile собирает приложение и раздаёт его через Nginx с поддержкой внутренних маршрутов. Аргумент сборки `VITE_API_BASE_URL` по умолчанию равен `https://todoapi.chkpnk.ru/api/v1`. Шрифт Manrope с кириллицей включён в сборку; внешние CDN для интерфейса не нужны.

Новый клиент включает «Сегодня», задачи, категории, Pomodoro, прогресс, достижения, историю, аккаунт, оплату и локальные награды. Монеты и инвентарь разделены по аккаунтам; старые общие данные браузера однократно получает первый вошедший аккаунт, исходные ключи сохраняются. Бустеры без серверного эффекта недоступны для покупки. Особенности текущего API и предложения развития: [docs/FRONTEND.md](docs/FRONTEND.md).

### 5. Swagger UI

```powershell
Set-Location docs
npm ci
node server.js
```

Откройте [http://localhost:8080](http://localhost:8080). Выберите сервер `Local backend` для локальных запросов. Для защищенных методов выполните `/login`, затем вставьте `access_token` в поле **Authorize**.

## Платежи и развертывание

Интеграция YooKassa включается при заданных `SHOP_ID` и `SECRET_KEY` в окружении backend. Без них `/payments/create` возвращает `503`. Сумма сейчас задана в обработчике: `49.00 RUB` за Премиум навсегда. Backend формирует URL возврата из `CLIENT_URL`; webhook провайдера должен указывать на `/api/v1/webhooks/yookassa`. Доступ к webhook ограничен IP YooKassa и loopback. Локально тариф можно переключить через `/dev/toggle-premium`; новый тариф проверяется по БД без обновления JWT.

`docker-compose.yml` предназначен для развертывания с готовыми образами `${IMAGE_BASE}-backend`, `${IMAGE_BASE}-email`, `${IMAGE_BASE}-activity-logger`, `${IMAGE_BASE}-frontend`, внешней PostgreSQL и внешней сетью Traefik `traefik`. Это не локальный Compose с секциями `build`. Он запускает MongoDB, сервисы приложения, Prometheus (`9090`) и Grafana (`3001`); backend и frontend публикуются через Traefik. Backend и email используют корневой `.env` и конфиги `prod.yaml`. Настройки деплоя также находятся в `ansible/`, `prometheus.yml` и `deploy/grafana/`.

## Тестирование

Полный backend-набор включает PostgreSQL-интеграцию и проверку гонок в CI.
Минимальное совместное покрытие рабочего кода `backend/` — 80%; отчёт сохраняется
как артефакт `backend-coverage`. Команды запуска и состав проверок:
[docs/BACKEND-TESTING.md](docs/BACKEND-TESTING.md).

Проверки фронтенда:

```powershell
Set-Location frontend
npm test
npm run build
npx playwright install chromium
npm run test:e2e
```

Браузерные тесты используют фикстуры текущего API, автоматически запускают Vite и проверяют размеры 360, 768 и 1440 px. Это проверка клиента; реальная доставка писем и платёжная система требуют отдельной проверки с настроенным backend.

Обычные тесты запускаются отдельно в каждом Go-модуле:

```powershell
Set-Location backend
go test ./... -count=1
Set-Location ../activity-logger
go test ./... -count=1
Set-Location ../notifiers
go test ./... -count=1
Set-Location email
go test ./... -count=1
```

Интеграционные проверки бизнес-логики и удаления запускаются на одноразовых PostgreSQL-контейнерах; требуется Docker:

```powershell
Set-Location backend
go test -count=1 -p 1 -tags integration ./internal/storage/postgres ./internal/http-server/router -run 'Test(FrontendContracts|BusinessActions|DeletionRelationships|APIContracts)$' -v
```

Проверяются параллельные и повторные запросы, выдача наград, владение задачами и сессиями, сохранение истории и откат при ошибке записи статистики. При нехватке памяти используйте `go test -p 1 ./...` для последовательной сборки пакетов.

E2E backend используют build tag `e2e` и testcontainers; требуется работающий Docker:

```powershell
Set-Location backend
go test -tags=e2e ./tests/e2e -count=1 -v
```

Вместо контейнера можно передать DSN отдельной тестовой PostgreSQL:

```powershell
$env:E2E_POSTGRES_DSN = 'postgres://postgres:postgres@localhost:5433/todo_e2e?sslmode=disable'
go test -tags=e2e ./tests/e2e -count=1 -v
```

Сценарий нагрузки запускается из корня и создает пользователей и задачи в указанном API; используйте тестовое окружение:

```powershell
go run ./scripts/loadtest -url http://localhost:8082 -c 10 -d 20s
```

## Лицензия

MIT, см. [LICENSE](LICENSE).
