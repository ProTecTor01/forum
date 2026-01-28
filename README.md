# Forum

Простой веб‑форум на Go с постами, комментариями, категориями и лайками/дизлайками. База данных — SQLite.

## Быстрый старт (Docker)

1) Запуск:
```bash
cd src
./runserver.sh 127.0.0.1:8080
```

2) Открыть:
`http://127.0.0.1:8080`

Данные сохраняются между перезапусками — база хранится в `src/assets/database` и монтируется в контейнер.

## Запуск без Docker

Нужен C‑компилятор (для SQLite).

Windows (PowerShell):
```powershell
$env:CGO_ENABLED=1; go run cmd/server/main.go
```
или
```powershell
.\run.ps1
```

Windows (CMD):
```cmd
set CGO_ENABLED=1 && go run cmd/server/main.go
```
или
```cmd
run.bat
```

Linux/macOS:
```bash
CGO_ENABLED=1 go run cmd/server/main.go
```

## Основные возможности

- Регистрация и вход
- Посты и комментарии
- Категории и фильтрация
- Лайки/дизлайки для постов и комментариев
- Вложенные комментарии (до 9 уровней)
- Синхронизация с Hacker News

Важно: при создании поста нужно выбрать хотя бы одну категорию.

## Основные маршруты

- `/posts` — список постов
- `/posts/<id>` — страница поста
- `/posts/create` — создать пост
- `/comments/create` — добавить комментарий
- `/categories` — список категорий
- `/categories/create` — создать категорию
- `/likes` — лайк/дизлайк
- `/sync/hackernews` — импорт из Hacker News (POST)

## Переменные окружения

- `PORT` — порт (по умолчанию 8080)
- `DB_PATH` — путь к БД (`./assets/database/forum.db`)
- `SCHEMA_PATH` — путь к схеме (`./assets/database/schema.sql`)
- `TEMPLATE_DIR` — путь к шаблонам (`./assets/templates/`)
- `STATIC_DIR` — путь к статике (`./assets/static/`)
- `FORCE_HTTPS` — Secure cookies (`false` по умолчанию)

## Примечания

- Для production рекомендуется HTTPS.
- Миграции выполняются автоматически при старте.
