# backend/Dockerfile
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Сначала копируем файлы для управления зависимостями
COPY go.work go.work.sum* ./
# Копируем go.mod и go.sum для обоих модулей (чтобы закешировать слои)
# Если каких-то файлов .sum нет, docker не упадет благодаря *
COPY backend/go.mod backend/go.sum* ./backend/
COPY notifiers/go.mod notifiers/go.sum* ./notifiers/
COPY notifiers/email/go.mod notifiers/email/go.sum* ./notifiers/email/

# Скачиваем зависимости
RUN cd backend && go mod download

# Копируем весь исходный код
COPY . .

# Собираем бэкенд
WORKDIR /app/backend
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/backend ./cmd/main.go

# Этап 2: Финальный образ
FROM alpine:3.19

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo/ /usr/share/zoneinfo/

WORKDIR /app

COPY --from=builder /go/bin/backend /usr/local/bin/backend

EXPOSE 8082

CMD ["backend"]