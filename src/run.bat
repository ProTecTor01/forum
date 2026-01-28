@echo off
set CGO_ENABLED=1
set SCRIPT_DIR=%~dp0
set DB_PATH=%SCRIPT_DIR%assets\database\forum.db
if not exist "%SCRIPT_DIR%assets\database" (
  mkdir "%SCRIPT_DIR%assets\database"
)
go run cmd/server/main.go
