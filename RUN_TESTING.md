# Руководство по запуску тестового стенда (Docker)

Данная конфигурация предназначена для тестирования и проверки проекта **ToDo Notificator**.  
Все сервисы поднимаются **одной командой** в изолированных Docker-контейнерах без необходимости локальной установки Go, Node.js или СУБД.

---

## Что входит в стенд

1. **Frontend React (React 19 + Vite + AntD)** из папки `frontend-react/todo-notificator` (доступен на [http://localhost:3000](http://localhost:3000))
2. **Frontend Alpine (Alpine.js + Tailwind)** из папки `frontend` (доступен на [http://localhost:3030](http://localhost:3030))
3. **Backend API (Go)** (доступен на [http://localhost:8082](http://localhost:8082), автоматически выполняет миграции БД)
4. **Activity Logger (Go + gRPC)** (порт `50051`)
5. **PostgreSQL 16** (порт `5432`, БД `todo`)
6. **MongoDB 7** (порт `27017`)

---

## Требования

- Установленный **Docker Desktop** (или Docker Engine + Docker Compose). Убедитесь, что Docker запущен.

---

## 🚀 Запуск одной командой

В корне проекта выполните команду:

```bash
docker compose -f docker-compose.local.yml up --build
```

> **Совет:** Чтобы запустить сервисы в фоновом режиме (демоном), добавьте флаг `-d`:
> ```bash
> docker compose -f docker-compose.local.yml up -d --build
> ```

---

## 🌐 Доступ к сервисам

- **React веб-интерфейс:** [http://localhost:3000](http://localhost:3000)
- **Alpine.js веб-интерфейс:** [http://localhost:3030](http://localhost:3030)
- **Бэкенд API:** [http://localhost:8082](http://localhost:8082)
  - Проверка работоспособности: [http://localhost:8082/api/v1/health](http://localhost:8082/api/v1/health)

---

## 🔍 Просмотр логов

Если контейнеры запущены в фоне (`-d`), логи можно смотреть командами:

- Все логи:
  ```bash
  docker compose -f docker-compose.local.yml logs -f
  ```
- Логи конкретного сервиса:
  ```bash
  docker compose -f docker-compose.local.yml logs -f backend
  docker compose -f docker-compose.local.yml logs -f frontend-react
  docker compose -f docker-compose.local.yml logs -f frontend-alpine
  docker compose -f docker-compose.local.yml logs -f activity-logger
  ```

---

## 🛑 Остановка стенда

Чтобы остановить все контейнеры:

```bash
docker compose -f docker-compose.local.yml down
```

Если вы хотите полностью очистить сохранённые данные в базах (начать с чистого листа):
```bash
docker compose -f docker-compose.local.yml down -v
```
