@echo off
cd /d "%~dp0"
call stop.bat >nul 2>&1
echo Сборка...
go build -o bin\psr.exe ./cmd/psr
if errorlevel 1 exit /b 1
rem HTTP-режим (cookie): 4с между запросами — меньше 403 от Cloudflare
set HLTV_REQUEST_DELAY=4
set HLTV_HUMAN_DELAY_MIN=0
set HLTV_HUMAN_DELAY_MAX=0
bin\psr.exe
