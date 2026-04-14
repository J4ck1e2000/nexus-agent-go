@echo off
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0knowledge-run.ps1" %*
endlocal
