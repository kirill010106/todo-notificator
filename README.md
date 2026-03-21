# 📋 To-Do Notificator

> **Умная система управления задачами с интеллектуальными уведомлениями**

A full-stack application for task management with smart multi-channel notifications (Email, Telegram, Matrix/Element). Built with **Go** backend, **Alpine.js** frontend, and **PostgreSQL** database.

![Status](https://img.shields.io/badge/status-active-success)
![Go](https://img.shields.io/badge/Go-1.24-blue)
![License](https://img.shields.io/badge/license-MIT-green)

---

## ✨ Features

### 📌 Task Management
- ✅ Create, read, update, delete tasks
- 📂 **Organize tasks by categories** (user-defined)
- 🏷️ Task statuses: Pending, Done
- 📅 Deadline tracking
- ⏰ Custom reminder times for each task
- 📝 Detailed task descriptions

### 🔔 Notifications
- 📧 **Email notifications** with HTML templates
- 🤖 ~~**Telegram bot** integration for instant notifications~~ (to be done)
- 💬 ~~**Matrix/Element messenger** integration~~ (to be done)
- 🎯 **Flexible scheduling**: Set custom reminder time for each task
- 🔄 Automated polling scheduler for timely delivery

### 👤 Authentication & Security
- 🔐 JWT-based authentication (Access & Refresh tokens)
- 🔒 Password hashing with security best practices
- 🛡️ Protected endpoints with middleware

### 📚 API Documentation
- 📖 Full **OpenAPI/Swagger** documentation
- 🎨 Interactive Swagger UI for testing endpoints
- 📊 Well-structured RESTful API

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Frontend (Alpine.js)                 │
│              HTML/CSS with JavaScript logic             │
└────────────────┬────────────────────────────────────────┘
                 │ HTTP
┌────────────────┴────────────────────────────────────────┐
│                  Backend (Go + Chi Router)              │
│  • Authentication (JWT)                                 │
│  • Task CRUD handlers                                   │
│  • Category management                                  │
│  • Category handlers                                    │
└────────────────┬────────────────────────────────────────┘
                 │ SQL
┌────────────────┴────────────────────────────────────────┐
│              Database (PostgreSQL)                      │
│  • users, tasks, categories, notifications              │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│         Notification Services (Separate Apps)           │
│  • Email Notifier (Go)                                  │
│  • Telegram Bot (Go)                                    │
│  • Matrix Bot (Go)                                      │
│  • Shared Scheduler & Storage Layer                     │
└─────────────────────────────────────────────────────────┘
```

---

## 🛠️ Tech Stack

### Backend
- **Language**: Go 1.24
- **Web Framework**: Chi Router (chi/v5)
- **Database**: PostgreSQL with pgx driver
- **Authentication**: JWT (golang-jwt)
- **Config**: cleanenv (environment-based config)
- **Migrations**: goose/v3
- **Logging**: slog (structured logging)

### Frontend
- **Framework**: Alpine.js
- **Styling**: Tailwind CSS (implied)
- **Template Engine**: HTML5

### Notification Services
- **Email**: Go with HTML templates
- **Telegram**: Go Telegram Bot API
- **Matrix**: Go Matrix API (gomatrix)
- **Scheduler**: Shared polling engine

### DevOps & Documentation
- **API Docs**: OpenAPI/Swagger
- **Version Control**: Git
- **Container**: Docker-ready

---

## 🚀 Quick Start

### Prerequisites
- Go 1.24+
- PostgreSQL 13+
- Node.js (for Swagger UI)
- Git

### Installation

1. **Clone the repository**
```bash
git clone https://github.com/kirill010106/todo-notificator.git
cd toDoNotificator
```

2. **Setup database**
```bash
# Create PostgreSQL database
createdb todo_notificator

# Run migrations
cd backend
go run cmd/main.go  # Migrations run automatically on startup
```

3. **Configure environment**
```bash
# Copy example config
cp backend/config/local.yaml backend/config/local.yaml

# Edit local.yaml with your settings:
# - Database credentials
# - JWT secret
# - Email/Telegram/Matrix API keys
```

4. **Start backend server**
```bash
cd backend
go run cmd/main.go
# Server runs on http://localhost:8080
```

5. **Start notification services**
```bash
# Email notifier
cd notifiers/email
go run cmd/main.go

# Telegram bot
cd notifiers/telegram-bot
go run cmd/main.go
```

6. **View API documentation**
```bash
cd docs
node server.js
# Swagger UI runs on http://localhost:3000
```

7. **Access frontend**
```
Open http://localhost:8080 in your browser
```

---

## 📖 API Endpoints

### Authentication
- `POST /auth/register` - Register new user
- `POST /auth/login` - Login user
- `POST /auth/logout` - Logout user
- `POST /auth/refresh` - Refresh access token

### Tasks
- `GET /tasks` - Get all user tasks
- `POST /tasks` - Create new task
- `GET /tasks/:id` - Get task by ID
- `PUT /tasks/:id` - Update task
- `DELETE /tasks/:id` - Delete task

### Categories
- `POST /categories` - Create category
- `GET /categories` - List categories (planned)
- `PUT /categories/:id` - Update category (planned)
- `DELETE /categories/:id` - Delete category (planned)

### Health
- `GET /health` - Health check endpoint

**Full API documentation**: See `docs/openapi.yaml` or open Swagger UI at `/docs`

---

## 🔐 Authentication

The application uses JWT (JSON Web Tokens) for stateless authentication:

1. **Register/Login** → Get access token & refresh token
2. **Access Token** → Valid for 1 hour, used for API requests
3. **Refresh Token** → Used to get new access token
4. **Protected Endpoints** → Require valid JWT in `Authorization: Bearer <token>` header

---

## 🔔 Notification System

### How It Works
1. **User creates task** with custom `reminder_at` time
2. **Scheduler polls** database every minute
3. **When reminder_at time arrives** → Task marked for notification
4. **Notification service** sends via Email/Telegram/Matrix
5. **Status updated** → Task marked as notified

### Configuration
Set notification channels in `config/local.yaml`:

```yaml
email:
  enabled: true
  smtp_host: smtp.gmail.com
  smtp_port: 587

telegram:
  enabled: true
  bot_token: "YOUR_BOT_TOKEN"

matrix:
  enabled: true
  homeserver: "https://matrix.org"
  bot_token: "YOUR_BOT_TOKEN"
```

---

## 🧪 Testing

Run tests for backend:

```bash
cd backend
go test ./...
go test ./... -v  # Verbose output
```

Unit tests include:
- ✅ Authentication handler tests
- ✅ Task CRUD tests
- ✅ Category creation tests
- ✅ Email formatting tests
- ✅ Storage layer tests

---

## 📋 Development Workflow

### Development Script
```bash
./dev.ps1
```

This PowerShell script helps with:
- Running migrations
- Starting backend server
- Running notification services
- Monitoring logs

### Code Style
- Go: Follow `go fmt` and `go vet`
- Structured logging with `slog`
- Error handling with context
- Tests for new features

---

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

---

## 📝 License

This project is licensed under the MIT License - see the LICENSE file for details.

---

## 👨‍💻 Author

**Kirill** - [GitHub](https://github.com/kirill010106)

---

## 🗺️ Roadmap

- [ ] Complete category CRUD endpoints (READ, UPDATE, DELETE)
- [ ] Category filtering for tasks
- [ ] Notification history & retry logic
- [ ] Task sharing between users
- [ ] Recurring tasks
- [ ] Mobile app
- [ ] Docker compose setup
- [ ] CI/CD pipeline

---

## ❓ FAQ

**Q: How do I change the database?**  
A: Edit the connection string in `config/local.yaml` and run migrations with `goose up`

**Q: Can I self-host?**  
A: Yes! Deploy backend, frontend, and notifier services on your own server or Docker container

**Q: How do I add a new notification channel?**  
A: Create a new service in `notifiers/` implementing the `Sender` interface in `shared/domain/domain.go`

---

## 📞 Support

For issues and questions:
🐛 [Open an issue](https://github.com/kirill010106/todo-notificator/issues)

---

**Made with ❤️ by Kirill**
