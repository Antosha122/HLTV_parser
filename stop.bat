@echo off
echo Остановка PSR...
taskkill /F /IM psr.exe >nul 2>&1
for /f "tokens=5" %%a in ('netstat -ano ^| findstr :8080 ^| findstr LISTENING') do (
  taskkill /F /PID %%a >nul 2>&1
)
echo Готово. Если Chrome остался открытым — закройте его вручную.
