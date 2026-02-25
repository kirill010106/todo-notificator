Write-Host "Starting all services..." -ForegroundColor Green

$backend = Start-Process powershell `
    -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot\backend'; air -c .air.toml" `
    -PassThru

$email = Start-Process powershell `
    -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot\notifiers\email'; air -c .air.toml" `
    -PassThru

Write-Host "Backend PID:        $($backend.Id)" -ForegroundColor Cyan
Write-Host "Email notifier PID: $($email.Id)" -ForegroundColor Cyan
Write-Host "Press Ctrl+C to stop all services" -ForegroundColor Yellow

try {
    Wait-Process -Id $backend.Id
} finally {
    Stop-Process -Id $backend.Id -ErrorAction SilentlyContinue
    Stop-Process -Id $email.Id   -ErrorAction SilentlyContinue
    Write-Host "All services stopped" -ForegroundColor Red
}