# frontend/Dockerfile
FROM nginx:alpine

# Копируем все файлы из папки frontend в стандартную папку для статики Nginx
COPY . /usr/share/nginx/html

# Заменяем дефолтный конфиг nginx на случай, если у вас роутинг через JS (хотя для Alpine.js обычно хватает дефолтного)
# Но базовая конфигурация Alpine Nginx и так отлично отдает index.html

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]