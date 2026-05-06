@echo off
setlocal enableextensions

set "SCRIPT_DIR=%~dp0"
set "SERVICE_DIR=%SCRIPT_DIR%.codex\runtime\browser-use-service"

echo [browser-use] 正在准备启动本地服务...

if not exist "%SERVICE_DIR%\src\server.js" (
  echo [browser-use] 错误：未找到服务入口 "%SERVICE_DIR%\src\server.js"
  echo [browser-use] 请确认当前脚本位于项目根目录。
  pause
  exit /b 1
)

where node >nul 2>nul
if errorlevel 1 (
  echo [browser-use] 错误：未检测到 Node.js，请先安装 Node.js 18+。
  pause
  exit /b 1
)

pushd "%SERVICE_DIR%" >nul
if errorlevel 1 (
  echo [browser-use] 错误：无法进入目录 "%SERVICE_DIR%"
  pause
  exit /b 1
)

if not exist "node_modules\playwright" (
  echo [browser-use] 首次运行，正在安装依赖...
  call npm install --no-fund --no-audit
  if errorlevel 1 (
    echo [browser-use] 错误：依赖安装失败。
    popd >nul
    pause
    exit /b 1
  )
)

echo [browser-use] 正在检查 Chromium 浏览器环境...
call npx playwright install chromium >nul 2>nul

echo [browser-use] 启动服务中...
start "browser-use-service" powershell -NoExit -Command "Set-Location '%SERVICE_DIR%'; npm start"

popd >nul

echo [browser-use] 服务启动命令已发送。
echo [browser-use] 访问健康检查：http://127.0.0.1:38765/health
echo [browser-use] 关闭此窗口不会影响已打开的服务终端。
exit /b 0
