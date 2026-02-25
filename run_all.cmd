@echo off
REM filepath: c:\Users\konst\GolandProjects\toDoNotificator\tmp\run_all.cmd

REM Запускаем backend в фоне
start "backend" cmd /c "cd backend && ..\tmp\backend"

REM Запускаем email notifier в фоне  
start "email-notifier" cmd /c "cd notifiers\email && ..\..\tmp\email-notifier"

REM Ждём Ctrl+C
echo All services started. Press Ctrl+C to stop.
pause > nul