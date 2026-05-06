@echo off
setlocal enableextensions

set "SCRIPT_DIR=%~dp0"
set "PROJECT_DIR=%SCRIPT_DIR%"
set "BROWSER_USE_DIR=%PROJECT_DIR%.codex\runtime\browser-use-service"

echo [all] 正在准备启动 Go 服务 + browser-use 服务...

if not exist "%PROJECT_DIR%main.go" (
  echo [all] 错误：未找到 "%PROJECT_DIR%main.go"
  echo [all] 请确认当前脚本位于项目根目录。
  pause
  exit /b 1
)

if not exist "%BROWSER_USE_DIR%\src\server.js" (
  echo [all] 错误：未找到 "%BROWSER_USE_DIR%\src\server.js"
  pause
  exit /b 1
)

where go >nul 2>nul
if errorlevel 1 (
  echo [all] 错误：未检测到 Go，请先安装 Go 1.25+。
  pause
  exit /b 1
)

where node >nul 2>nul
if errorlevel 1 (
  echo [all] 错误：未检测到 Node.js，请先安装 Node.js 18+。
  pause
  exit /b 1
)

pushd "%BROWSER_USE_DIR%" >nul
if errorlevel 1 (
  echo [all] 错误：无法进入 browser-use 目录。
  pause
  exit /b 1
)

if not exist "node_modules\playwright" (
  echo [all] browser-use 首次运行，正在安装依赖...
  call npm install --no-fund --no-audit
  if errorlevel 1 (
    echo [all] 错误：browser-use 依赖安装失败。
    popd >nul
    pause
    exit /b 1
  )
)

echo [all] 正在检查 browser-use Chromium 环境...
call npx playwright install chromium >nul 2>nul

echo [all] 正在启动 browser-use 服务...
start "browser-use-service" powershell -NoExit -Command "Set-Location '%BROWSER_USE_DIR%'; npm start"

popd >nul

echo [all] 正在启动 Go 服务...
start "chatadd-go-service" powershell -NoExit -Command "Set-Location '%PROJECT_DIR%'; go run ."

echo [all] 启动命令已发送。
echo [all] Go 页面地址：http://127.0.0.1:18473/
echo [all] browser-use 健康检查：http://127.0.0.1:38765/health
echo [all] 若首次编译较慢，请等待 3-10 秒后再刷新页面。
exit /b 0
