$env:CGO_ENABLED=1
$dbDir = Join-Path $PSScriptRoot "assets\\database"
if (-not (Test-Path $dbDir)) {
    New-Item -ItemType Directory -Path $dbDir | Out-Null
}
$env:DB_PATH = Join-Path $dbDir "forum.db"
go run cmd/server/main.go
