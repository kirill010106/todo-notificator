param(
    [switch]$SkipDatabases,
    [switch]$SkipFrontend
)

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding           = [System.Text.Encoding]::UTF8
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

$mongoContainer = "todo-mongodb"
$mongoPort      = 27017

# Applications parse the root file themselves; never evaluate secrets as shell code.
# Database endpoints and config paths come from .env, without script overrides.
$env:ENV_FILE = Join-Path $PSScriptRoot '.env'
if (-not (Test-Path -LiteralPath $env:ENV_FILE -PathType Leaf)) {
    throw "Создайте корневой .env из .env.example перед запуском."
}
if (-not (Get-Command air -ErrorAction SilentlyContinue)) {
    throw "Air не найден в PATH."
}

$frontendDir = Join-Path $PSScriptRoot 'frontend'
$viteEntry = Join-Path $frontendDir 'node_modules\vite\bin\vite.js'
if (-not $SkipFrontend) {
    if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
        throw "Node.js не найден в PATH. Установите Node.js или используйте -SkipFrontend."
    }
    if (-not (Test-Path -LiteralPath $viteEntry -PathType Leaf)) {
        throw "Не установлены зависимости frontend. Выполните npm ci в папке frontend или используйте -SkipFrontend."
    }
}

# Windows PowerShell 5.1 turns native stderr into ErrorRecord objects even with
# 2>&1. Capture them with a local preference and decide by the native exit code.
function Invoke-Docker {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [switch]$AllowFailure
    )
    $ErrorActionPreference = 'Continue'
    $output = @(& docker @Arguments 2>&1)
    $exitCode = $LASTEXITCODE
    $message = ($output | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "Ошибка Docker (код ${exitCode}): $message"
    }
    return [pscustomobject]@{ ExitCode = $exitCode; Output = $message }
}

# --- Универсальная функция запуска контейнеров ---
function Ensure-Container {
    param(
        [string]$Name,
        [scriptblock]$RunCommand,
        [scriptblock]$HealthCheck,
        [string]$SuccessMessage
    )

    # A failed daemon request is not evidence that the container is missing.
    $containers = Invoke-Docker -Arguments @('container', 'ls', '--all', '--filter', "name=^/${Name}$", '--format', '{{.Names}}')
    if ($Name -notin ($containers.Output -split '\r?\n')) {
        Write-Host "Creating container '$Name'..." -ForegroundColor Yellow
        & $RunCommand
    } else {
        $state = Invoke-Docker -Arguments @('inspect', '-f', '{{.State.Running}}', $Name)
        if ($state.Output.Trim() -eq 'false') {
            Write-Host "Starting existing container '$Name'..." -ForegroundColor Yellow
            $null = Invoke-Docker -Arguments @('start', $Name)
        } elseif ($state.Output.Trim() -eq 'true') {
            Write-Host "Container '$Name' is already running." -ForegroundColor DarkGray
        } else {
            throw "Не удалось определить состояние контейнера '$Name': $($state.Output)"
        }
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
if (-not $SkipDatabases -and -not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker не найден в PATH. Убедитесь, что Docker Desktop запущен."
}

if (-not $SkipDatabases) {
    Write-Host "=== Подготовка локальных баз данных ===" -ForegroundColor Cyan
    Write-Host "Ожидаем готовности Docker Engine..." -ForegroundColor Gray
    for ($attempt = 1; $attempt -le 30; $attempt++) {
        $engine = Invoke-Docker -Arguments @('info', '--format', '{{.OSType}}') -AllowFailure
        if ($engine.ExitCode -eq 0) { break }
        if ($attempt -lt 30) { Start-Sleep -Seconds 1 }
    }
    if ($engine.ExitCode -ne 0) {
        throw "Docker Engine недоступен. Дождитесь запуска Docker Desktop и повторите .\dev.ps1. Если ошибка 500 сохраняется, перезапустите Docker Desktop. Для уже работающих внешних баз используйте -SkipDatabases. Ошибка Docker: $($engine.Output)"
    }
    if ($engine.Output.Trim() -ne 'linux') {
        throw "Для локальных баз нужны Linux-контейнеры. Переключите Docker Desktop в режим Linux containers."
    }

    # --- 2. Запуск PostgreSQL ---
    Ensure-Container -Name $pgContainer `
        -RunCommand {
            $null = Invoke-Docker -Arguments @('run', '--name', $pgContainer,
                '-e', "POSTGRES_USER=$pgUser", '-e', "POSTGRES_PASSWORD=$pgPass",
                '-e', "POSTGRES_DB=$pgDb", '-p', "127.0.0.1:${pgPort}:5432",
                '-d', 'postgres:16-alpine')
        } `
        -HealthCheck {
            $health = Invoke-Docker -Arguments @('exec', $pgContainer, 'pg_isready', '-U', $pgUser, '-d', $pgDb) -AllowFailure
            return ($health.ExitCode -eq 0)
        } `
        -SuccessMessage "Postgres готов: 127.0.0.1:$pgPort"

    # --- 3. Запуск MongoDB ---
    Ensure-Container -Name $mongoContainer `
        -RunCommand {
            $null = Invoke-Docker -Arguments @('run', '--name', $mongoContainer,
                '-p', "127.0.0.1:${mongoPort}:27017", '-d', 'mongo:7')
        } `
        -HealthCheck {
            $health = Invoke-Docker -Arguments @('exec', $mongoContainer, 'mongosh', '--eval', "db.adminCommand('ping')", '--quiet') -AllowFailure
            return ($health.ExitCode -eq 0)
        } `
        -SuccessMessage "MongoDB готова: 127.0.0.1:$mongoPort"
}

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
Set-Location '$backendDir'
air -c .air.toml
"@

$emailCmd = @"
Set-Location '$emailDir'
air -c .air.toml
"@

$activityCmd = @"
Set-Location '$activityLoggerDir'
air -c .air.toml
"@

$backendProc  = Start-Process powershell -ArgumentList "-NoExit", "-Command", $backendCmd -PassThru
$emailProc    = Start-Process powershell -ArgumentList "-NoExit", "-Command", $emailCmd -PassThru
$activityProc = Start-Process powershell -ArgumentList "-NoExit", "-Command", $activityCmd -PassThru

Write-Host "Backend PID:         $($backendProc.Id)" -ForegroundColor Cyan
Write-Host "Email notifier PID:  $($emailProc.Id)" -ForegroundColor Cyan
Write-Host "Activity Logger PID: $($activityProc.Id)" -ForegroundColor Cyan

# --- 6. Ожидание и Graceful остановка дочерних окон ---
$frontendProc = $null
try {
    if (-not $SkipFrontend) {
        $frontendLogDir = Join-Path $PSScriptRoot 'tmp\frontend'
        New-Item -ItemType Directory -Path $frontendLogDir -Force | Out-Null
        $frontendOutput = Join-Path $frontendLogDir 'vite.log'
        $frontendError = Join-Path $frontendLogDir 'vite-error.log'

        # Start Node directly so this process is Vite itself, without an npm/cmd
        # parent leaving the dev server alive after Stop-Process.
        $frontendProc = Start-Process node -WorkingDirectory $frontendDir `
            -ArgumentList @("`"$viteEntry`"", '--host', '127.0.0.1', '--port', '3030', '--strictPort') `
            -WindowStyle Hidden -RedirectStandardOutput $frontendOutput `
            -RedirectStandardError $frontendError -PassThru

        Write-Host "Frontend PID:        $($frontendProc.Id)" -ForegroundColor Cyan
        Write-Host "Frontend: http://127.0.0.1:3030 (изменения обновляются автоматически)" -ForegroundColor Green
        Write-Host "Журналы frontend: $frontendLogDir" -ForegroundColor Gray
    }

    Write-Host "`nНажмите Ctrl+C в этом окне для остановки всех сервисов..." -ForegroundColor Yellow
    while (-not $backendProc.HasExited) {
        if ($null -ne $frontendProc -and $frontendProc.HasExited) {
            throw "Frontend завершился (код $($frontendProc.ExitCode)). Проверьте $frontendError. Если порт 3030 занят, остановите прежний frontend или используйте -SkipFrontend."
        }
        Start-Sleep -Milliseconds 500
    }
} finally {
    if ($null -ne $frontendProc -and -not $frontendProc.HasExited) {
        Stop-Process -Id $frontendProc.Id -Force -ErrorAction SilentlyContinue
    }
    Stop-Process -Id $backendProc.Id, $emailProc.Id, $activityProc.Id -Force -ErrorAction SilentlyContinue
    Get-Process app -ErrorAction SilentlyContinue | 
        Where-Object { $_.Path -like "*toDoNotificator*" -or $_.Path -like "*tmp\activity-logger*" } | 
        Stop-Process -Force -ErrorAction SilentlyContinue
    Write-Host "Все сервисы остановлены." -ForegroundColor Red
}
