# frontend-react

React-клиент для ToDo Notificator (Vite + TypeScript + Zustand + Axios).

Этот клиент является альтернативой основному Alpine.js фронтенду из ../frontend.

## Стек

- React 19
- TypeScript
- Vite
- Zustand
- Axios
- Tailwind CSS

## Запуск

```bash
npm install
npm run dev
```

По умолчанию Vite поднимается на http://localhost:5173.

## Доступные команды

```bash
npm run dev      # локальный dev-сервер
npm run build    # production build
npm run preview  # локальный просмотр production build
npm run lint     # eslint
```

## API endpoint

Сейчас базовый URL захардкожен в src/api/client.ts:

- http://localhost:8082/api/v1

Если backend работает на другом адресе, обновите src/api/client.ts.

## Аутентификация

- access_token и refresh_token хранятся в localStorage
- Axios interceptor автоматически добавляет Bearer token
- При 401 выполняется попытка refresh через /refresh

## Статус

Клиент рабочий, но в деплой-контуре проекта основным по-прежнему является frontend (Alpine.js).
