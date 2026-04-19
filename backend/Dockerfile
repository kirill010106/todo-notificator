# backend/Dockerfile
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.work go.work.sum* ./
COPY go.mod go.sum* ./
COPY activity-logger/go.mod activity-logger/go.sum* ./activity-logger/
COPY backend/go.mod backend/go.sum* ./backend/
COPY notifiers/go.mod notifiers/go.sum* ./notifiers/
COPY notifiers/email/go.mod notifiers/email/go.sum* ./notifiers/email/

RUN cd backend && go mod download

COPY . .

WORKDIR /app/backend
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/backend ./cmd

FROM alpine:3.19

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo/ /usr/share/zoneinfo/

WORKDIR /app

COPY --from=builder /go/bin/backend /usr/local/bin/backend

EXPOSE 8082

CMD ["backend"]
