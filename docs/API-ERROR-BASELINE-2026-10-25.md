# Нулевой этап: фактический контракт ошибок HTTP API

Дата: 25 октября 2026 года. База: HEAD `6c32fb8`, текущая рабочая копия. Основание: этап 0 из [отчёта по обработке ошибок](API-ERROR-HANDLING-REPORT-2026-10-25.md).

Цель этапа — зафиксировать ответы до изменения способа их формирования. Рабочие обработчики, middleware, схема БД и frontend не изменены. Несогласованности текущего контракта сохранены в ожиданиях явно; их устранение потребует отдельного обоснованного изменения тестов и документации.

## Что добавлено и зачем

| Изменение | Обоснование | Чем защищено |
|---|---|---|
| [error_baseline_test.go](../backend/internal/http-server/router/error_baseline_test.go) | Зафиксировать итог обработки ошибок настоящим `router.New`, включая глобальные middleware, rate limit и исключения webhook | 38 сценариев полного ответа и отдельная проверка идентификатора текущего запроса |
| [error_contract_baseline.json](../backend/internal/http-server/router/testdata/error_contract_baseline.json) | Хранить проверяемые ожидания независимо от реализации нормализации | Сравниваются HTTP-статус, `Content-Type`, `Retry-After`, выбранные дополнительные заголовки, весь JSON либо точные байты текста; проверяются соответствие числа fixtures числу сценариев, наличие ожиданий и уникальность имён |
| [error_baseline_integration_test.go](../backend/internal/http-server/router/error_baseline_integration_test.go) | Проверить ошибки, зависящие от актуального аккаунта, тарифов, транзакций и сохранённого ключа | 27 сценариев через настоящий роутер и PostgreSQL 16 с применением миграций; отдельная проверка отката после ошибки напоминаний |
| [error_contract_postgres_baseline.json](../backend/internal/http-server/router/testdata/error_contract_postgres_baseline.json) | Зафиксировать контракт команд и предметных отказов, включая типы полей и legacy-формат | Те же полные проверки ответа; каждый сценарий получает отдельного пользователя |
| Этот документ | Связать маршрут, условие, наблюдаемый ответ и проверку, чтобы дальнейшие изменения можно было оценивать по конкретным последствиям | Матрица из 65 сценариев ниже соответствует именам тестов и fixtures |
| [Исходный отчёт](API-ERROR-HANDLING-REPORT-2026-10-25.md) | Добавить переход к результатам нулевого этапа и отделить исходный статический анализ от последующих исполненных проверок | Ссылки проверены; исходные рекомендации сохранены |

JSON-сравнение учитывает весь объект, включая дополнительные и отсутствующие поля, строковый/числовой `status`, `null` и `{}`. Порядок ключей и пробелы JSON не фиксируются. Текстовые и пустые тела, а также два выбранных успешных ответа сравниваются побайтно.

Ожидания подготовлены явно, а не записаны автоматически из текущего ответа. При рефакторинге не следует автоматически перегенерировать fixtures после падения теста: сначала нужно определить и объяснить изменение контракта.

## Уровни проверки и ограничения

Набор `TestErrorContractBaseline` исполняет рабочий роутер, обработчики, middleware и реальный PostgreSQL-адаптер поверх `sqlmock`. Ошибки БД задаются на конкретных операциях; это проверяет обработку отказа приложением, но не исполнение SQL сервером PostgreSQL.

Набор `TestErrorContractPostgresBaseline` создаёт одноразовый контейнер `postgres:16-alpine`, применяет миграции и исполняет SQL в настоящей БД. Каждый сценарий использует нового пользователя и новый экземпляр роутера, поэтому состояние и бюджеты rate limit не переносятся между сценариями. Базы разработки и production не используются.

В проверках webhook используется реальный SDK YooKassa с локальной реализацией HTTP-транспорта в памяти. Запрос к провайдеру не выходит в сеть; количество вызовов транспорта проверяется. Это проверка приложения и SDK, а не настоящего платёжного сервиса. SMTP и activity-logger не подключаются.

Panic вызывается на трёх маршрутах, добавленных только тестом после `router.New`. Они проходят действующую глобальную цепочку middleware; рабочих маршрутов или дефектов приложения эти тесты не добавляют. Использован стандартный logging hook chi для подсчёта ожидаемого panic без печати полного стека.

Запросы передаются роутеру через `httptest.ResponseRecorder`, без HTTP-сокета. В таблицах зафиксированы заголовки, записанные приложением. Автоматические заголовки сетевого HTTP-сервера, например `Date`, вычисляемый `Content-Length` или определение типа содержимого транспортом, не входят в этот baseline. Для `/ping` приложение на recorder оставляет `Content-Type` пустым; это не утверждение об отсутствии заголовка у ответа настоящего сетевого сервера.

Матрица покрывает все действующие способы формирования ошибок, перечисленные в исходном отчёте, и основные границы совместимости. Она не является исчерпывающим перечнем всех отказов каждого API-метода. При переносе конкретного обработчика следует добавлять отсутствующие предметные сценарии до изменения его реализации.

## Обозначения формы ответа

| Обозначение | Поля и типы |
|---|---|
| J | `status: "Error"`, `code`, `message`, совместимый `error == message`, `request_id`, `field_errors: {}` |
| N | Как J, но `status` — число, равное HTTP-статусу |
| V | Как J, но непустые одинаковые объекты `field_errors` и `validation_errors` |
| F | Как J, но непустой `field_errors`, без `validation_errors` |
| Z | Как J, но `field_errors: null` |
| O | `code`, `message`, `error`, `request_id`, `field_errors: {}`; ключа `status` нет |
| T | Текстовое тело, JSON-полей нет |
| E | Пустое тело, JSON-полей нет |

Для J/N/V/F/Z/O `Content-Type` равен `application/json`. Для T — `text/plain; charset=utf-8`, кроме отдельно обозначенного случая panic после записи. Точные сообщения и объекты полей находятся в JSON-fixtures; они также проверяются тестами.

Для JSON-ошибок входящий `X-Request-Id` фиксируется как `error-baseline-request` и должен появиться в теле без подмены. Отдельный тест проверяет два разных заданных идентификатора и два автоматически созданных идентификатора. В последнем случае проверяется непустое и различающееся значение.

Для ответов, изначально сформированных через `http.Error`, зафиксирован `X-Content-Type-Options: nosniff`, в том числе после JSON-нормализации 429. У выбранного роутерного 405 текущий пользовательский обработчик оставляет `Allow` пустым; это также зафиксировано как исходное состояние, без исправления в нулевом этапе.

## Матрица: роутер, авторизация и входные данные

Эти сценарии входят в `TestErrorContractBaseline`. В сценариях с отказом БД используется `sqlmock`; остальные завершаются до обращения к БД.

| Имя сценария | Маршрут и условие | HTTP | Код | Форма |
|---|---|---:|---|---|
| `router_not_found` | `GET /api/v1/unknown`, маршрут отсутствует | 404 | `NOT_FOUND` | N |
| `router_method_not_allowed` | `POST /ping`, метод не зарегистрирован | 405 | `METHOD_NOT_ALLOWED` | N |
| `dev_route_absent` | `POST /api/v1/dev/toggle-premium`, окружение prod | 404 | `NOT_FOUND` | N |
| `auth_missing` | `GET /api/v1/tasks`, нет Authorization | 401 | `UNAUTHORIZED` | J |
| `auth_header_malformed` | Тот же маршрут, схема Basic | 401 | `UNAUTHORIZED` | J |
| `auth_token_invalid` | Тот же маршрут, невалидный Bearer | 401 | `UNAUTHORIZED` | J |
| `login_empty` | `POST /api/v1/login`, пустое тело | 400 | `VALIDATION_ERROR` | J |
| `login_malformed` | Тот же маршрут, оборванный JSON | 400 | `VALIDATION_ERROR` | J |
| `login_field_validation` | Тот же маршрут, `{}` | 400 | `VALIDATION_ERROR` | V, поля email/password |
| `login_body_limit` | Тот же маршрут, JSON больше 4096 байт | 413 | `PAYLOAD_TOO_LARGE` | J |
| `refresh_body_limit` | `POST /api/v1/refresh`, JSON больше 4096 байт | 413 | `PAYLOAD_TOO_LARGE` | J |
| `register_malformed` | `POST /api/v1/register`, оборванный JSON | 400 | `VALIDATION_ERROR` | J |
| `register_password_validation` | Тот же маршрут, пароль короче минимума | 400 | `VALIDATION_ERROR` | J |
| `forgot_email_validation` | `POST /api/v1/password/forgot`, невалидный email | 400 | `VALIDATION_ERROR` | J |
| `login_unknown_account` | `POST /api/v1/login`, SQL возвращает отсутствие пользователя | 401 | `UNAUTHORIZED` | J, `invalid credentials` |
| `login_database_error` | Тот же маршрут, отказ чтения пользователя | 500 | `INTERNAL_ERROR` | J, `internal error` |
| `auth_account_unavailable` | `GET /api/v1/tasks`, валидный JWT, отказ чтения текущего аккаунта | 503 | `ACCOUNT_UNAVAILABLE` | J |
| `health_database_error` | `GET /api/v1/health`, отказ PingContext | 503 | `INTERNAL_ERROR` | J |
| `account_database_error` | `POST /api/v1/register`, отказ начала транзакции | 500 | `INTERNAL_ERROR` | J, `Не удалось выполнить запрос` |
| `problem_database_error` | `GET /api/v1/me/focus`, успешная auth, отказ начала snapshot | 500 | `INTERNAL_ERROR` | J, `Не удалось выполнить запрос` |
| `operation_database_error` | `POST /api/v1/tasks`, успешная auth, отказ начала транзакции команды | 500 | `INTERNAL_ERROR` | J, `Не удалось сохранить действие` |

## Матрица: инфраструктурные ответы, webhook и успешные ответы

Эти сценарии также входят в `TestErrorContractBaseline`. Для budget-тестов создаётся отдельный роутер; все предыдущие запросы должны пройти соответствующий порог без 429. Проверяется именно следующий запрос, а не искусственный вызов обработчика лимита.

| Имя сценария | Маршрут и условие | HTTP | Код или точное тело | Форма / значимые заголовки |
|---|---|---:|---|---|
| `public_rate_limit` | `GET /ping`, 101-й запрос с одного IP | 429 | `RATE_LIMITED` | O, `Retry-After: 60` |
| `login_rate_limit` | `POST /api/v1/login`, 21-й запрос | 429 | `RATE_LIMITED` | O, `Retry-After: 60` |
| `invalid_auth_rate_limit` | `GET /api/v1/tasks` без JWT, 21-й запрос | 429 | `RATE_LIMITED` | O, `Retry-After: 60` |
| `webhook_rate_limit` | `POST /api/v1/webhooks/yookassa`, 301-й запрос | 429 | `Too Many Requests` и перевод строки | T, `Retry-After: 60` |
| `webhook_forbidden` | Webhook с недоверенного адреса `198.51.100.25` | 403 | `forbidden` и перевод строки | T |
| `webhook_gateway_unavailable` | Webhook с loopback, gateway отключён | 503 | `payment verification is temporarily unavailable` и перевод строки | T |
| `webhook_malformed` | Webhook с оборванным JSON, gateway-заглушка включён | 400 | `unexpected EOF` и перевод строки | T, вызовов провайдера нет |
| `webhook_ignored_event` | Событие `payment.waiting_for_capture` | 200 | Пусто | E, `Content-Type` отсутствует, вызовов провайдера нет |
| `webhook_missing_payment_id` | `payment.succeeded` без ID | 400 | `missing payment id` и перевод строки | T, вызовов провайдера нет |
| `webhook_provider_error` | `payment.succeeded`, транспорт провайдера возвращает ошибку | 502 | `payment verification failed` и перевод строки | T, один вызов провайдера |
| `webhook_succeeded_database_error` | Провайдер подтверждает succeeded, БД не начинает транзакцию | 500 | Пусто | E, `Content-Type` отсутствует, один вызов провайдера |
| `webhook_canceled_database_error` | Провайдер подтверждает canceled, SQL изменения статуса возвращает ошибку | 500 | `failed to update payment status` и перевод строки | T, один вызов провайдера |
| `panic_before_write` | Тестовый `/__baseline/panic`, panic до ответа | 500 | `INTERNAL_ERROR`, сообщение `Internal Server Error` | O |
| `panic_after_write` | Тот же тестовый маршрут, panic после записи тела | 200 | `partial response` | Точное тело, `Content-Type: text/plain` |
| `webhook_panic_before_write` | Тестовый `/__baseline/webhooks/panic`, panic до ответа | 500 | Пусто | E, `Content-Type` отсутствует |
| `success_ping` | `GET /ping` | 200 | `pong` | Точное тело; на recorder `Content-Type` отсутствует |
| `success_api_info` | `GET /api/v1` без завершающего слеша | 200 | Сведения о сервисе | Точный успешный JSON с переводом строки, `application/json` |

Panic после начала ответа с текущей цепочкой оставляет 200 и частичное тело. Это зафиксированное поведение синтетического сценария, а не обещание корректного завершения запроса при любом panic. На этапе замены middleware нельзя молча считать этот случай эквивалентным JSON-ошибке 500.

## Матрица: предметные ошибки с настоящим PostgreSQL

Набор `TestErrorContractPostgresBaseline`. Все защищённые маршруты проходят рабочую auth с актуальными данными аккаунта из БД; публичные маршруты используют действующие обработчики.

| Имя сценария | Маршрут и условие | HTTP | Код | Форма |
|---|---|---:|---|---|
| `auth_account_missing` | `GET /api/v1/tasks`, пользователь JWT удалён из БД | 401 | `UNAUTHORIZED` | J, `account not found` |
| `auth_version_changed` | Тот же маршрут, версия авторизации увеличена в БД | 401 | `UNAUTHORIZED` | J, `Войдите в аккаунт заново` |
| `register_duplicate` | `POST /api/v1/register`, занятый email | 409 | `EMAIL_EXISTS` | J |
| `verify_invalid_token` | `GET /api/v1/verify?token=invalid` | 400 | `INVALID_TOKEN` | J |
| `profile_empty` | `PATCH /api/v1/me/profile`, `{}` | 400 | `VALIDATION_ERROR` | J |
| `today_invalid_pagination` | `GET /api/v1/me/today?limit=0` | 400 | `VALIDATION_ERROR` | J |
| `tasks_invalid_pagination` | `GET /api/v1/tasks?limit=0` | 400 | `VALIDATION_ERROR` | J |
| `category_missing` | `GET /api/v1/categories/999999`, категория отсутствует | 404 | `NOT_FOUND` | J |
| `category_limit` | `POST /api/v1/categories`, у Free уже три категории | 403 | `CATEGORY_LIMIT_REACHED` | J, код назначает нормализатор по тексту |
| `category_duplicate` | Тот же маршрут, повтор существующего имени | 409 | `CONFLICT` | J |
| `command_key_limit` | `POST /api/v1/tasks`, ключ из 129 символов | 400 | `INVALID_IDEMPOTENCY_KEY` | J |
| `command_body_limit` | Тот же маршрут, тело больше 1 МиБ | 413 | `BODY_TOO_LARGE` | J |
| `command_malformed_task` | Тот же маршрут, оборванный JSON | 400 | `INVALID_TASK_REQUEST` | J |
| `command_legacy_reminder` | Тот же маршрут, удалённое поле `reminder_at` | 400 | `INVALID_TASK_REQUEST` | J |
| `command_reminders_null` | Тот же маршрут, `reminders: null` | 400 | `REMINDER_VALIDATION_ERROR` | F, поле `reminders` |
| `command_reminder_field` | Тот же маршрут, дробный `offset_minutes` | 400 | `REMINDER_VALIDATION_ERROR` | F, поле `reminders.0` |
| `command_title_validation` | Тот же маршрут, `{}` внутри транзакционного исполнителя | 400 | `VALIDATION_ERROR` | V, поле `title` |
| `command_reminder_limit` | Тот же маршрут, три корректных напоминания на задачу у Free; отказ внутри SaveTask | 403 | `REMINDER_LIMIT_REACHED` | F, поле `reminders`; запись через `RenderContractError` |
| `command_task_missing` | `PATCH /api/v1/tasks/999999`, задача отсутствует | 404 | `NOT_FOUND` | J |
| `command_key_conflict` | `POST /api/v1/tasks`, ключ прежнего успешного создания с другим телом | 409 | `IDEMPOTENCY_KEY_REUSED` | Z |
| `pause_exhausted` | `POST /api/v1/pomodoros/{id}/pause`, перерыв уже использован | 409 | `BREAK_ALREADY_USED` | J, код назначает нормализатор по тексту |
| `shop_malformed` | `POST /api/v1/me/shop/actions`, неизвестное поле | 400 | `VALIDATION_ERROR` | O |
| `shop_unknown_action` | Тот же маршрут, неизвестное действие | 400 | `INVALID_SHOP_ACTION` | J |
| `cancel_invalid_id` | `POST /api/v1/pomodoros/invalid/cancel` | 400 | `VALIDATION_ERROR` | O |
| `cancel_session_missing` | `POST /api/v1/pomodoros/999999/cancel`, сессия отсутствует | 404 | `SESSION_NOT_FOUND` | J |
| `payments_disabled` | `POST /api/v1/payments/create`, gateway отключён | 503 | `INTERNAL_ERROR` | J |
| `user_rate_limit` | `GET /api/v1/tasks?limit=0`, 101-й запрос аккаунта | 429 | `RATE_LIMITED` | O, `Retry-After: 60` |

Для конфликтующего ключа сначала действительно создаётся задача через HTTP с этим ключом. Для исчерпанного перерыва создаётся сессия и используется её перерыв в настоящей БД. Эти условия не подменены возвратом заранее выбранной предметной ошибки.

В `command_reminder_limit` проверяется также откат после промежуточной вставки задачи: в PostgreSQL не должны остаться задача, факт дневной активности от триггера или сохранённый результат команды. Этот набор всё равно не является полной проверкой транзакционных гарантий. Успешный replay, after-commit действия и конкурирующие команды дополнительно проверяются существующим `TestFrontendContracts` в [storage/postgres/contracts_integration_test.go](../backend/internal/storage/postgres/contracts_integration_test.go). Для будущего изменения `Operation` нужны и эти проверки с настоящей БД.

## Значимые различия, которые теперь обнаружат тесты

- Замена числового `status` в роутерных 404/405 на строку изменит N-fixtures.
- Добавление `status` в ответы 429, некорректное действие магазина или отмену с невалидным ID изменит O-fixtures.
- Нормализация `field_errors: null` до `{}` изменит Z-fixture конфликта ключа.
- Удаление `validation_errors` либо изменение имён полей изменит V-fixtures; удаление `error` изменит все соответствующие JSON-fixtures.
- Изменение текста лимита категорий или исчерпанного перерыва без явного назначения специального кода обнаружится через изменение итогового ответа.
- Изменение формата, пустого тела или HTTP-статуса webhook обнаружится отдельно от обычных JSON-ошибок.
- Потеря `Retry-After`, изменение порога любого из пяти бюджетов или попадание webhook под JSON-нормализацию обнаружится соответствующим budget-тестом.
- Удаление глобального middleware без замены источников ошибок изменит состав полей и поведение восстановления после panic.

Текущие 503 от health и отключённых платежей имеют общий `INTERNAL_ERROR`, тогда как ошибка чтения актуального аккаунта — отдельный `ACCOUNT_UNAVAILABLE`. Улучшение этой классификации в будущем считается изменением кодов, а не только внутренним упрощением.

## Команды проверки

Из `backend/`:

```powershell
# Быстрый набор без Docker: настоящий роутер, локальные заглушки.
go test -p 1 ./internal/http-server/router -run '^TestErrorContract' -count=1

# Оба новых набора: дополнительно одноразовый PostgreSQL с миграциями.
$env:DOCKER_HOST = 'npipe:////./pipe/dockerDesktopLinuxEngine'
go test -p 1 -tags integration ./internal/http-server/router -run '^TestErrorContract' -count=1
```

Полный backend-прогон с PostgreSQL, `-race` и нормализованным покрытием выполняется по [BACKEND-TESTING.md](BACKEND-TESTING.md). В этой работе используется отдельный игнорируемый профиль `backend/coverage-error-baseline.out`, чтобы не перезаписывать прежний `coverage.out`. Успешный целевой прогон выполнен обычным запуском Go с системным временным каталогом. В промежуточных запусках с проектным `GOTMPDIR` наблюдался отказ доступа тестовых процессов к Docker named pipe, хотя Docker CLI оставался доступен; точная причина ограничения отдельно не установлена.

## Результаты проверки

- Окончательные 65 сценариев и отдельная проверка идентификатора запроса прошли с `-race`. 27 сценариев исполнены с PostgreSQL 16 в одноразовом Docker-контейнере; подтверждён откат после отказа лимита напоминаний.
- Полный backend-прогон: успешно, `-count=1 -p 1 -race -tags integration -covermode=atomic`, с покрытием всех рабочих backend-пакетов без сгенерированных mocks. Исполнены также существующие проверки настоящего роутера, миграций, отката и конкурентных команд в PostgreSQL.
- Нормализованное покрытие: **83,63% — 4454/5326 операторов**, порог **80%** пройден. Проверка выполнена `go run -p 1 ../scripts/coveragecheck -profile coverage-error-baseline.out -min 80`; собственные тесты `scripts/coveragecheck` также прошли.
- Окружение успешного финального прогона: Windows/amd64, Go 1.27.1, Docker Desktop с сервером 29.7.2, одноразовые PostgreSQL 16. Это локальный результат, а не утверждение о запуске удалённого CI.
- Промежуточные прогоны с исчезнувшими временными `vet.cfg` и отказом доступа к Docker не засчитаны как успешные. После возврата к обычному запуску Go полный набор завершился с кодом 0; тесты приложения и настройки окружения проекта для обхода ошибок не менялись.
- Проверены форматирование Go, отсутствие ошибок `git diff --check`, ссылки документов и соответствие всех 65 сценариев матрице.
- Frontend и остальные Go-модули не изменялись; отдельные прогоны их наборов в рамках этого этапа не требуются.

Следующий этап — общий рендерер и адаптеры совместимости. Менять рабочее поведение в рамках нулевого этапа не требуется.
