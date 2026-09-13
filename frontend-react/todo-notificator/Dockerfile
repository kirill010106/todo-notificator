FROM node:20-slim AS builder

WORKDIR /app

COPY . .

# Если папка dist уже существует — пропускаем установку и сборку.
# Если dist отсутствует (чистый клон из Git) — собираем внутри контейнера.
RUN if [ ! -d "dist" ] || [ ! -f "dist/index.html" ]; then \
      npm install --no-audit --no-fund && npm run build; \
    fi

FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
