$ErrorActionPreference = "Stop"

$localDbContainer = "todo-local-pg-alt"
$localDbPort = 55432
$localDbName = "todo"
$localDbUser = "postgres"
$localDbPassword = "postgres"
$localDbURL = "postgres://{0}:{1}@127.0.0.1:{2}/{3}?sslmode=disable" -f $localDbUser, $localDbPassword, $localDbPort, $localDbName

$localMongoContainer = "todo-mongodb"
$localMongoPort = 27017
$localMongoURL = "mongodb://127.0.0.1:{0}" -f $localMongoPort

function Ensure-LocalPostgres {
    param(
        [string]$Container,
        [int]$Port,
        [string]$Database,
        [string]$User,
        [string]$Password
    )

    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw "docker is not available in PATH"
    }

    $containerId = docker ps -aq --filter "name=^/$Container$"
    if ($LASTEXITCODE -ne 0) {
        throw "failed to query docker containers"
    }

    if (-not $containerId) {
        Write-Host "Creating local Postgres container '$Container' on port $Port..." -ForegroundColor Yellow
        docker run --name $Container `
            -e "POSTGRES_USER=$User" `
            -e "POSTGRES_PASSWORD=$Password" `
            -e "POSTGRES_DB=$Database" `
            -p "${Port}:5432" `
            -d postgres:16-alpine | Out-Null

        if ($LASTEXITCODE -ne 0) {
            throw "failed to create local postgres container. Port $Port may be in use"
        }
    } else {
        $isRunning = docker inspect -f "{{.State.Running}}" $Container 2>$null
        if ($LASTEXITCODE -ne 0) {
            throw "failed to inspect docker container '$Container'"
        }

        if ($isRunning -ne "true") {
            Write-Host "Starting local Postgres container '$Container'..." -ForegroundColor Yellow
            docker start $Container | Out-Null
            if ($LASTEXITCODE -ne 0) {
                throw "failed to start local postgres container '$Container'"
            }
        }
    }

    for ($attempt = 1; $attempt -le 30; $attempt++) {
        docker exec $Container pg_isready -U $User -d $Database | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Local Postgres is ready on 127.0.0.1:$Port" -ForegroundColor Green
            return
        }
        Start-Sleep -Milliseconds 500
    }

    throw "local postgres container '$Container' did not become ready in time"
}

function Ensure-LocalMongo {
    param(
        [string]$Container,
        [int]$Port
    )

    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw "docker is not available in PATH"
    }

    $containerId = docker ps -aq --filter "name=^/$Container$"
    if ($LASTEXITCODE -ne 0) {
        throw "failed to query docker containers"
    }

    if (-not $containerId) {
        Write-Host "Creating local Mongo container '$Container' on port $Port..." -ForegroundColor Yellow
        docker run --name $Container -p "${Port}:27017" -d mongo:7 | Out-Null
        if ($LASTEXITCODE -ne 0) {
            throw "failed to create local mongo container. Port $Port may be in use"
        }
    } else {
        $isRunning = docker inspect -f "{{.State.Running}}" $Container 2>$null
        if ($LASTEXITCODE -ne 0) {
            throw "failed to inspect docker container '$Container'"
        }
        if ($isRunning -ne "true") {
            Write-Host "Starting local Mongo container '$Container'..." -ForegroundColor Yellow
            docker start $Container | Out-Null
            if ($LASTEXITCODE -ne 0) {
                throw "failed to start local mongo container '$Container'"
            }
        }
    }
    
    # Simple wait
    for ($attempt = 1; $attempt -le 20; $attempt++) {
        docker exec $Container mongosh --eval "db.adminCommand('ping')" --quiet | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Local Mongo is ready on 127.0.0.1:$Port" -ForegroundColor Green
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "local mongo container '$Container' did not become ready in time"
}

Write-Host "Preparing local database..." -ForegroundColor Green
Ensure-LocalPostgres -Container $localDbContainer -Port $localDbPort -Database $localDbName -User $localDbUser -Password $localDbPassword
Ensure-LocalMongo -Container $localMongoContainer -Port $localMongoPort

Write-Host "Starting all services on local DB..." -ForegroundColor Green
Write-Host "DATABASE_URL=$localDbURL" -ForegroundColor DarkGray
Write-Host "MONGO_URL=$localMongoURL" -ForegroundColor DarkGray

$backendDir = Join-Path $PSScriptRoot "backend"
$emailDir = Join-Path $PSScriptRoot "notifiers\email"
$activityLoggerDir = Join-Path $PSScriptRoot "activity-logger"

$backendCommand = @"
`$env:DATABASE_URL = '$localDbURL'
`$env:CONFIG_PATH = '.\config\local.yaml'
Set-Location '$backendDir'
air -c .air.toml
"@

$emailCommand = @"
`$env:DATABASE_URL = '$localDbURL'
`$env:EMAIL_CONFIG_PATH = '.\config\local.yaml'
Set-Location '$emailDir'
air -c .air.toml
"@

$activityLoggerCommand = @"
`$env:MONGO_URL = '$localMongoURL'
Set-Location '$activityLoggerDir'
air -c .air.toml
"@

$backend = Start-Process powershell `
    -ArgumentList "-NoExit", "-Command", $backendCommand `
    -PassThru

$email = Start-Process powershell `
    -ArgumentList "-NoExit", "-Command", $emailCommand `
    -PassThru

$activityLogger = Start-Process powershell `
    -ArgumentList "-NoExit", "-Command", $activityLoggerCommand `
    -PassThru

Write-Host "Backend PID:         $($backend.Id)" -ForegroundColor Cyan
Write-Host "Email notifier PID:  $($email.Id)" -ForegroundColor Cyan
Write-Host "Activity Logger PID: $($activityLogger.Id)" -ForegroundColor Cyan
Write-Host "Press Ctrl+C to stop all services" -ForegroundColor Yellow

try {
    Wait-Process -Id $backend.Id
} finally {
    Stop-Process -Id $backend.Id -ErrorAction SilentlyContinue
    Stop-Process -Id $email.Id   -ErrorAction SilentlyContinue
    Stop-Process -Id $activityLogger.Id   -ErrorAction SilentlyContinue
    Write-Host "All services stopped" -ForegroundColor Red
}