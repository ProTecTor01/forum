# Real-Time Forum

Лёгкий форум в виде SPA на Go + SQLite + WebSocket (реальное время).

Коротко
- Сервер: Go
- База: SQLite (файловая база в assets/database)
- Клиент: SPA в assets/static
- WebSocket: пуш-уведомления (новые посты, комментарии, ЛС, присутствие)

Требования
- Go 1.24+ (рекомендуется)
- На Windows для сборки `github.com/mattn/go-sqlite3` нужен C-компилятор (MSYS2 / MinGW-w64)

Быстрая подготовка (Windows)
1. Установите Go: https://go.dev/dl/
2. Установите MSYS2: https://www.msys2.org/
   - В MSYS2 MinGW 64-bit выполните:
     ```powershell
     pacman -Syu
     pacman -S mingw-w64-x86_64-gcc
     ```
   - Добавьте `C:\msys64\mingw64\bin` в PATH (если путь отличается — используйте свой).
3. Убедитесь, что `go version` и `gcc --version` работают в PowerShell.

Запуск локально
- Запускайте из папки `src` (там находится `go.mod`).

PowerShell (Windows):
```powershell
cd src
$env:CGO_ENABLED=1
go run cmd/server/main.go
```

Одинарная команда:
```powershell
$env:CGO_ENABLED=1; Set-Location src; go run cmd/server/main.go
```

WSL / Linux / macOS:
```bash
cd src
CGO_ENABLED=1 go run cmd/server/main.go
```

Полезные переменные окружения
- `PORT` — порт (по умолчанию 8080)
- `DB_PATH` — путь к sqlite (по умолчанию `./assets/database/forum.db`)
- `SCHEMA_PATH` — путь к `schema.sql`
- `STATIC_DIR` — директория со статикой (`./assets/static/`)
- `FORCE_HTTPS` — `true/false` (влияет на secure cookies)

Пример с другим портом:
```powershell
cd src
$env:CGO_ENABLED=1
$env:PORT=8081
go run cmd/server/main.go
```

Сборка бинарника:
```bash
cd src
CGO_ENABLED=1 go build -o forum cmd/server/main.go
./forum
```

Инициализация БД
- В `assets/database` есть `schema.sql` и миграции. Если файл БД отсутствует, сервер может создать его и применить схему (следите за логами).

Docker (опционально)
```bash
cd src
docker build -t forum .
docker run -p 8080:8080 -e PORT=8080 forum
```

Где смотреть код
- Сервер: [src/cmd/server/main.go](src/cmd/server/main.go)
- Конфиг: [src/internal/config](src/internal/config)
- Статика: [src/assets/static](src/assets/static)
- БД и миграции: [src/assets/database](src/assets/database)

Частые проблемы
- Ошибка сборки sqlite3/cgo — нет `gcc` в PATH. Установите MSYS2/MinGW-w64 и добавьте `mingw64/bin` в PATH.
- `cannot find module` — вы запускали из корня проекта; перейдите в `src`.
- Порт занят — измените `PORT`.

Если нужно — могу помочь настроить MSYS2 и запустить сервер прямо сейчас; пришлите вывод ошибок из консоли.

---
Резервная копия старой версии: `README.md.bak`
