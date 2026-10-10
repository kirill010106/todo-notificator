[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding           = [System.Text.Encoding]::UTF8
$ErrorActionPreference = "Stop"

$ErrorActionPreference = "Stop"
# Отключаем падение PowerShell от обычных сообщений Docker в stderr
if (Test-Path Variable:PSNativeCommandUseErrorActionPreference) {
    $PSNativeCommandUseErrorActionPreference = $false
}

# --- Конфигурация локального окружения ---
$pgContainer = "todo-local-pg-alt"
$pgPort      = 5433
$pgDb        = "todo"
$pgUser      = "postgres"
$pgPass      = "postgres"
$dbUrl       = "postgres://${pgUser}:${pgPass}@127.0.0.1:${pgPort}/${pgDb}?sslmode=disable"

$mongoContainer = "todo-mongodb"
$mongoPort      = 27017
$mongoUrl       = "mongodb://127.0.0.1:${mongoPort}"

# --- Универсальная функция запуска контейнеров ---
function Ensure-Container {
    param(
        [string]$Name,
        [scriptblock]$RunCommand,
        [scriptblock]$HealthCheck,
        [string]$SuccessMessage
    )

    $status = (docker inspect -f '{{.State.Running}}' $Name 2>$null)

    if ($null -eq $status) {
        Write-Host "Creating container '$Name'..." -ForegroundColor Yellow
        & $RunCommand
    } elseif ($status -ne "true") {
        Write-Host "Starting existing container '$Name'..." -ForegroundColor Yellow
        docker start $Name | Out-Null
    } else {
        Write-Host "Container '$Name' is already running." -ForegroundColor DarkGray
    }

    # Ожидание готовности
    Write-Host "Waiting for '$Name' to be ready..." -ForegroundColor Gray
    for ($i = 1; $i -le 30; $i++) {
        $ok = & $HealthCheck
        if ($ok) {
            Write-Host $SuccessMessage -ForegroundColor Green
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "Container '$Name' failed to become ready in time."
}

# --- 1. Проверка наличия Docker ---
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker не найден в PATH. Убедитесь, что Docker Desktop запущен."
}

Write-Host "=== Подготовка локальных баз данных ===" -ForegroundColor Cyan

# --- 2. Запуск PostgreSQL ---
Ensure-Container -Name $pgContainer `
    -RunCommand {
        docker run --name $pgContainer `
            -e "POSTGRES_USER=$pgUser" `
            -e "POSTGRES_PASSWORD=$pgPass" `
            -e "POSTGRES_DB=$pgDb" `
            -p "${pgPort}:5432" `
            -d postgres:16-alpine | Out-Null
    } `
    -HealthCheck {
        $null = docker exec $pgContainer pg_isready -U $pgUser -d $pgDb 2>$null
        return ($LASTEXITCODE -eq 0)
    } `
    -SuccessMessage "Postgres готов: 127.0.0.1:$pgPort"

# --- 3. Запуск MongoDB ---
Ensure-Container -Name $mongoContainer `
    -RunCommand {
        docker run --name $mongoContainer `
            -p "${mongoPort}:27017" `
            -d mongo:7 | Out-Null
    } `
    -HealthCheck {
        $null = docker exec $mongoContainer mongosh --eval "db.adminCommand('ping')" --quiet 2>$null
        return ($LASTEXITCODE -eq 0)
    } `
    -SuccessMessage "MongoDB готова: 127.0.0.1:$mongoPort"

# --- 4. Очистка старых зависших процессов app.exe ---
Get-Process app -ErrorAction SilentlyContinue | 
    Where-Object { $_.Path -like "*toDoNotificator*" -or $_.Path -like "*tmp\activity-logger*" } | 
    Stop-Process -Force -ErrorAction SilentlyContinue

# --- 5. Запуск микросервисов через Air ---
Write-Host "`n=== Запуск микросервисов (Air) ===" -ForegroundColor Cyan

$backendDir        = Join-Path $PSScriptRoot "backend"
$emailDir          = Join-Path $PSScriptRoot "notifiers\email"
$activityLoggerDir = Join-Path $PSScriptRoot "activity-logger"

$backendCmd = @"
`$env:DATABASE_URL = '$dbUrl'
`$env:CONFIG_PATH   = '.\config\local.yaml'
Set-Location '$backendDir'
air -c .air.toml
"@

$emailCmd = @"
`$env:DATABASE_URL        = '$dbUrl'
`$env:EMAIL_CONFIG_PATH   = '.\config\local.yaml'
Set-Location '$emailDir'
air -c .air.toml
"@

$activityCmd = @"
`$env:MONGO_URL = '$mongoUrl'
Set-Location '$activityLoggerDir'
air -c .air.toml
"@

$backendProc  = Start-Process powershell -ArgumentList "-NoExit", "-Command", $backendCmd -PassThru
$emailProc    = Start-Process powershell -ArgumentList "-NoExit", "-Command", $emailCmd -PassThru
$activityProc = Start-Process powershell -ArgumentList "-NoExit", "-Command", $activityCmd -PassThru

Write-Host "Backend PID:         $($backendProc.Id)" -ForegroundColor Cyan
Write-Host "Email notifier PID:  $($emailProc.Id)" -ForegroundColor Cyan
Write-Host "Activity Logger PID: $($activityProc.Id)" -ForegroundColor Cyan
Write-Host "`nНажмите Ctrl+C в этом окне для остановки всех сервисов..." -ForegroundColor Yellow

# --- 6. Ожидание и Graceful остановка дочерних окон ---
try {
    Wait-Process -Id $backendProc.Id
} finally {
    Stop-Process -Id $backendProc.Id, $emailProc.Id, $activityProc.Id -Force -ErrorAction SilentlyContinue
    Get-Process app -ErrorAction SilentlyContinue | 
        Where-Object { $_.Path -like "*toDoNotificator*" -or $_.Path -like "*tmp\activity-logger*" } | 
        Stop-Process -Force -ErrorAction SilentlyContinue
    Write-Host "Все сервисы остановлены." -ForegroundColor Red
}