# Real-Time Forum

SPA форум на Go + SQLite + WebSockets. Один HTML, все страницы переключаются через JS.

## Запуск

Windows (PowerShell):
```powershell
$env:CGO_ENABLED=1; go run cmd/server/main.go
```

Linux/macOS:
```bash
CGO_ENABLED=1 go run cmd/server/main.go
```

Открыть:
`http://127.0.0.1:8080`

## Что есть

- Регистрация и вход (ник или email + пароль)
- Лента постов с категориями
- Комментарии к постам
- Личные сообщения (реал?тайм)
- Онлайн/оффлайн пользователей
- Сортировка чатов по последнему сообщению
- Непрочитанные сообщения + «прочитать все»
- Баннер новых постов

## WebSocket события

Сервер отправляет события в формате:
```json
{ "type": "...", "data": { } }
```

Типы:
- `post_created`
- `comment_created`
- `pm_message`
- `presence`
- `typing`

## API маршруты

- `POST /api/register`
- `POST /api/login`
- `POST /api/logout`
- `GET /api/me`

- `GET /api/posts`
- `GET /api/posts/:id`
- `POST /api/posts`
- `POST /api/comments`
- `POST /api/likes`

- `GET /api/categories`
- `POST /api/categories`

- `GET /api/chats`
- `GET /api/messages`
- `POST /api/messages`

WebSocket:
- `GET /ws`

## Переменные окружения

- `PORT` — порт (по умолчанию 8080)
- `DB_PATH` — путь к БД (`./assets/database/forum.db`)
- `SCHEMA_PATH` — путь к схеме (`./assets/database/schema.sql`)
- `STATIC_DIR` — путь к статике (`./assets/static/`)
- `FORCE_HTTPS` — Secure cookies (`false` по умолчанию)
