# notifiers/email/Dockerfile
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.work go.work.sum* ./
COPY backend/go.mod backend/go.sum* ./backend/
COPY notifiers/go.mod notifiers/go.sum* ./notifiers/
COPY notifiers/email/go.mod notifiers/email/go.sum* ./notifiers/email/

RUN cd notifiers/email && go mod download

COPY . .

# Переходим в директорию email и собираем её
WORKDIR /app/notifiers/email
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/email-notifier ./cmd/main.go

# Этап 2
FROM alpine:3.19

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo/ /usr/share/zoneinfo/

WORKDIR /app

COPY --from=builder /go/bin/email-notifier /usr/local/bin/email-notifier

EXPOSE 8081

CMD ["email-notifier"]