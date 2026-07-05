@echo off
cd /d "%~dp0"
echo.
echo === PSR: первая загрузка данных с HLTV ===
echo.
echo 1. Откройте https://www.hltv.org в браузере
echo 2. F12 -^> Network -^> обновите страницу -^> любой запрос -^> Cookie
echo 3. Вставьте cookie ниже когда программа спросит
echo.
set /p HLTV_COOKIE=Cookie: 
set HLTV_USE_BROWSER=false
set HLTV_COOKIE=%HLTV_COOKIE%
echo.
echo Поиск команд Natus Vincere...
go run ./cmd/psr teams -search "navi"
echo.
echo Поиск команд FaZe...
go run ./cmd/psr teams -search "faze"
echo.
set /p TEAM1=ID команды 1 (например 4608): 
set /p TEAM2=ID команды 2 (например 6667): 
echo.
echo Загрузка матчей за 3 месяца... Это займет 15-30 минут.
go run ./cmd/psr sync teams -team1 %TEAM1% -team2 %TEAM2% -months 3
echo.
echo Готово! Теперь запустите run.bat
pause
