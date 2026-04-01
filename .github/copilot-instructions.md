# Project Guidelines

## Code Style
- **Go Version:** 1.24
- **Frameworks:** `go-chi/chi/v5` for routing, `slog` for structured logging, `cleanenv` for configs.
- **Logging Pattern:** Always declare `const op = "package.function"` at the start of a handler or service function. Use `log = log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))` where applicable to provide operation paths.
- **Dependency Injection:** Use constructor functions (e.g., `New(log *slog.Logger, provider Interface, cfg *config.Config) http.HandlerFunc`) for components. Inject dependencies via interfaces defined local to the consumer.
- **Naming & Scope:** HTTP handlers are grouped by feature with nested directories (e.g., `internal/http-server/handlers/auth/login`). Keep `Request`/`Response` DTOs scoped tightly to their respective handler package.

## Architecture
- **Monorepo Setup:** 
  - `backend` (Go REST API)
  - `frontend` (Alpine.js HTML/JS)
  - `notifiers` (Go microservices: `email`, `telegram-bot`, `shared`).
- **Layered Design:**
  - **Domain:** Pure value struct objects (`internal/domain/*.go`). No business logic.
  - **Storage:** Database interactions via `pgx` and standard `database/sql` (`internal/storage/postgres`).
  - **Handlers:** Use `http.HandlerFunc`. Parse JSON, validate structs using `go-playground/validator/v10`, delegate to injected injected interfaces, and return standard outputs via `lib/api/response`.
  - **Config:** Environment-based YAML configs mapped to structs via `cleanenv` (`internal/config`).

## Build and Test
- **Dev Server:** Run `.\dev.ps1` from the repository root to start `backend` and `notifiers/email` simultaneously with `air` hot-reloading.
- **Database:** Migrations run automatically with `goose` on startup. 
- **Testing Standard:** 
  - Run tests with `go test ./...` in the respective module.
  - Use `stretchr/testify` (`require`) for assertions.
  - Test HTTP handlers using `httptest.NewRequest`/`httptest.NewRecorder` and interface mocks (defined alongside tests).
  - Test the storage layer with `DATA-DOG/go-sqlmock` using regex query matching and strong row assertions. Always end with `mock.ExpectationsWereMet()`.

## Conventions
- **Error Handling:** Propagate errors gracefully without panicking. Prepend operation context to dynamic errors (e.g., `fmt.Errorf("%s: %w", op, err)`). Use standard JSON errors via `response.Error()`.
- **Frontend Stack:** Vanilla HTML/CSS with Alpine.js logic. No complex bundlers needed; served directly.
- **API Docs:** Follow OpenAPI/Swagger standards. Keep `docs/openapi.yaml` manually updated when adding or modifying endpoints, testing locally via `node server.js` from `docs`.
