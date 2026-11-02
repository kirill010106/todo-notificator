# Обработка ошибок: третий этап

Дата: 25 октября 2026 года. Основание: этап 3 [плана](API-ERROR-HANDLING-REPORT-2026-10-25.md), [baseline](API-ERROR-BASELINE-2026-10-25.md) и [результаты миграции](API-ERROR-MIGRATION-2026-10-25.md).

Rate limit и восстановление после panic используют общий рендерер. Формат существующих ответов сохраняется: обычный 429 и recoverer/500 не получают legacy `status`; webhook сохраняет текстовый 429 и пустой 500. `Errors` ещё подключён — его удаление относится к этапу 4.

## Изменения и защита

| Файл | Основание и проверка |
|---|---|
| [apiresponse/webhook.go](../backend/internal/http-server/apiresponse/webhook.go), новый | Общий предикат исключения webhook, повторяющий прежний `strings.Contains(path, "/webhooks/")`. Он используется тремя участниками, чтобы политика не расходилась. Новые публичные маршруты не вводятся; оба семейства ответов проверяются baseline |
| [middleware/contracts/contracts.go](../backend/internal/http-server/middleware/contracts/contracts.go) | Только переход на общий предикат. Буферизация `Errors`, эвристики и исполнение `Operation` в этом этапе не меняются |
| [middleware/ratelimit/ratelimit.go](../backend/internal/http-server/middleware/ratelimit/ratelimit.go) | `WithLimitHandler` записывает полный JSON 429 напрямую. `httprate` по-прежнему определяет бюджеты и заголовки; `Retry-After`, `X-RateLimit-*` и nosniff сохранены. Webhook использует прежний `http.Error` |
| [ratelimit_test.go](../backend/internal/http-server/middleware/ratelimit/ratelimit_test.go) | Без нормализатора проверены JSON/text, поле request_id, статус, лимит/остаток бюджета, Retry-After и nosniff. Существующие тесты отдельных аккаунтов за общим IP и IPv6 остаются |
| [recoverer/recoverer.go](../backend/internal/http-server/middleware/recoverer/recoverer.go), новый | Перед началом финального ответа — полный безопасный JSON 500, после записи/Flush/ReadFrom либо успешного Hijack — без второго тела. `ErrAbortHandler` пробрасывается. Остаточные Content-Length/Content-Encoding удаляются перед новым 500. Диагностика остаётся в chi LogEntry, если он задан; иначе один структурированный panic-log содержит request_id и stack |
| [recoverer_test.go](../backend/internal/http-server/middleware/recoverer/recoverer_test.go), новый | Проверены первый ответ, отложенные заголовки, частичное тело, финальный статус без тела, Flush, webhook, ErrAbortHandler, ReadFrom, успешный/неуспешный Hijack, наличие/отсутствие необязательных интерфейсов writer. Через локальный net/http-сервер проверен 103 перед финальным 500; это отдельная проверка recoverer, без глобального буфера |
| [backend/go.mod](../backend/go.mod) | Уже имеющийся `httpsnoop v1.0.4` объявлен прямой зависимостью. Он сохраняет набор интерфейсов исходного writer; самостоятельная копия комбинаций Flush/Hijack/ReaderFrom/Pusher не нужна. Версия и go.sum не менялись |
| [router/router.go](../backend/internal/http-server/router/router.go) | chi Recoverer заменён на новый recoverer на прежнем месте, внутри логирования и метрик. Порядок остальных middleware и маршруты прежние |
| [error_baseline_test.go](../backend/internal/http-server/router/error_baseline_test.go), [integration_test.go](../backend/internal/http-server/router/error_baseline_integration_test.go) | Убраны пять исключений инфраструктурных сценариев из режима без нормализатора. Теперь все 65 сценариев исполняются в обоих режимах, включая все бюджеты rate limit, panic и исключения webhook |
| [infrastructure_errors_test.go](../backend/internal/http-server/router/infrastructure_errors_test.go), новый | Через тот же настоящий router проверены HTTP-статус, единственный request-log и panic-log с текущим request_id, а также приращение соответствующего Prometheus-счётчика. Проверены 500 до записи и 200 после частичного тела/Flush, в обоих режимах нормализатора |
| [frontend/api.ts](../frontend/src/lib/api.ts) | Добавлен тип `ApiErrorData`: необязательные поля допускают прежние payload; nullable field_errors и варианты status названы явно. Логика fetch, refresh, сессии, сообщений и повторных команд не менялась |
| [api.contract.test.ts](../frontend/src/lib/api.contract.test.ts), новый | Frontend читает 52 JSON-ошибки из неизменённых backend-fixtures. Дополнительно проверены старый error-only, числовой status, uppercase ERROR, validation_errors, неизвестный code и независимость сообщения от code. Для старого и текущего payload проверены сохранение ключа команды после 429/503 и его удаление после 400 |
| [openapi.yaml](openapi.yaml) | Общие code/message/error/request_id/field_errors обязательны; status необязателен и допускает Error/HTTP-число; field_errors:null явно разрешён для legacy. APIError ссылается на тот же контракт. У всех 48 операций описаны 429/500; обычный 429 использует общий ответ с Retry-After, webhook — text/plain |
| [test_api_error_contract.py](../scripts/test_api_error_contract.py), новый | Проверка согласованности YAML с 52 JSON-формами, обязательности и типов общих полей, Retry-After у всех 48 операций и разрешимости внутренних ссылок. Используется уже применяемый в scripts PyYAML; это адресная проверка контракта, а не полный валидатор OpenAPI |
| Этот документ и [исходный план](API-ERROR-HANDLING-REPORT-2026-10-25.md) | Фиксируют защиту изменений, фактический результат и границы этапа; прежние отчёты остаются историческими |

## Решения о совместимости

- `field_errors:null` не заменяется на `{}` незаметно для клиента. Схема теперь описывает существующий вариант; рендерер по умолчанию по-прежнему выдаёт `{}`. Отказ от legacy null потребует отдельного изменения контракта.
- Коды 429/500, публичные тексты и заголовки совпадают с baseline. Хеши двух файлов fixtures до/после этапа совпадают; ожидаемые ответы под новую реализацию не переписывались.
- Заголовок запроса `Connection: Upgrade` сам по себе больше не подавляет ответ 500. Решение зависит от фактической записи или успешного Hijack. Для обычных маршрутов это устраняет исключение прежнего chi Recoverer; отдельный тест защищает этот случай.
- Для Flush/ReadFrom фиксируется начало ответа; при Flush до тела явно отправляется прежний неявный 200. Это позволяет логированию и метрикам видеть фактический статус, не дописывая JSON после отправки заголовков.
- Глобальный `Errors` всё ещё буферизует ответы. Проверка recoverer с информационным 103 относится к его самостоятельной работе; потоковые режимы всего приложения в рамках этого этапа не заявляются. Удаление глобального буфера — следующий этап.
- Смешанные версии проверены через реальные клиентские функции с заглушкой fetch и прежними fixtures. Отдельное развёртывание двух версий frontend/backend не выполнялось. Совместимость прежнего клиента поддержана сохранением публичных payload и алиасов; эвристики frontend пока остаются.
- Авторизация, SQL, транзакции, порядок блокировок, идемпотентность платежей и бизнес-правила не менялись. Реальные провайдеры и SMTP не вызывались, деплой не выполнялся.

## Проверки

- Целевой backend-запуск с `-p 1 -race -tags integration -count=1`: recoverer, ratelimit и router прошли. Проверены **130 baseline-сценариев** — 65 с `Errors` и 65 без него; **54** из них используют настоящий PostgreSQL в одноразовых Docker-контейнерах. Дополнительно прошли проверки статусов, логов, метрик и writer-интерфейсов.
- `python -m unittest discover -s scripts -p 'test_api_error_contract.py' -v`: **3 проверки прошли**.
- Из `frontend/`: `npm test` — **119 тестов в 12 файлах прошли**; `npm run build` — успешно, включая TypeScript. Поведение пользовательских экранов не менялось, Playwright повторно не запускался.
- Полный backend-набор выполняется по [BACKEND-TESTING.md](BACKEND-TESTING.md): `-count=1 -p 1 -race -tags integration -covermode=atomic`, с coverpkg всех рабочих пакетов без generated mocks. Отдельные игнорируемые артефакты: `backend/coverage-error-infrastructure.out` и `.workbench/error-baseline-20261025/backend-infrastructure.log`.

Полный backend-прогон завершился с кодом 0: все пакеты, `-race`, PostgreSQL, миграции и существующие проверки откатов/конкурентных команд прошли. `go run -p 1 ../scripts/coveragecheck -profile coverage-error-infrastructure.out -min 80` прошёл: **84,17% — 4403/5231 операторов**, требуемые 80% достигнуты.

Проверены форматирование десяти Go-файлов этого этапа, `git diff --check`, ссылки документов и неизменность двух baseline-fixtures. Версии зависимостей, go.sum и frontend package-lock не менялись; только роль httpsnoop в go.mod стала прямой. Результаты относятся к локальному запуску, не к удалённому CI.

Проверки остальных Go-модулей не запускались: их код не изменялся. Коммитов и PR не создавалось.
