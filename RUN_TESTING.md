# Запуск тестового стенда

Проект ToDo Notificator запускается в Docker Compose. Локальная установка Go, Node.js, PostgreSQL и MongoDB не требуется.

## Состав стенда

| Сервис | Назначение | Адрес на хосте |
|---|---|---|
| `frontend-alpine` | Статический Alpine.js/Tailwind CSS клиент под Nginx | `http://localhost:3030` |
| `backend` | REST API на Go | `http://localhost:8082` |
| `activity-logger` | gRPC-сервис аудита | `localhost:50051` |
| `postgres` | PostgreSQL 16, БД `todo` | `localhost:5432` |
| `mongo` | MongoDB 7 для журнала активности | `localhost:27017` |

React-клиент и сервисы email-уведомлений в текущий стенд не входят. Порт `3000` проектом не используется.

## Требования

- Docker Desktop либо Docker Engine с Docker Compose v2.
- Свободные порты `3030`, `8082`, `50051`, `5432` и `27017`.

## Запуск

```bash
docker compose -f docker-compose.local.yml up -d --build
```

Проверка состояния:

```bash
docker compose -f docker-compose.local.yml ps
```

- Фронтенд: [http://localhost:3030](http://localhost:3030)
- Backend API: [http://localhost:8082](http://localhost:8082)
- Healthcheck: [http://localhost:8082/api/v1/health](http://localhost:8082/api/v1/health)

## Логи

```bash
docker compose -f docker-compose.local.yml logs -f
docker compose -f docker-compose.local.yml logs -f backend
docker compose -f docker-compose.local.yml logs -f frontend-alpine
docker compose -f docker-compose.local.yml logs -f activity-logger
docker compose -f docker-compose.local.yml logs -f postgres
docker compose -f docker-compose.local.yml logs -f mongo
```

## Остановка

```bash
docker compose -f docker-compose.local.yml down
```

Полная очистка данных стенда:

```bash
docker compose -f docker-compose.local.yml down -v
```

> `down -v` необратимо удаляет локальные данные PostgreSQL и MongoDB.

## Тесты Backend

```bash
go test ./backend/...
go test -tags=e2e ./backend/tests/e2e/...
```
