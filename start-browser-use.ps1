param()

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$serviceDir = Join-Path $scriptDir ".codex\runtime\browser-use-service"

Write-Host "[browser-use] 正在准备启动本地服务..."

if (-not (Test-Path (Join-Path $serviceDir "src\server.js"))) {
  Write-Host "[browser-use] 错误：未找到服务入口 $serviceDir\src\server.js"
  exit 1
}

if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
  Write-Host "[browser-use] 错误：未检测到 Node.js，请先安装 Node.js 18+。"
  exit 1
}

Push-Location $serviceDir

if (-not (Test-Path (Join-Path $serviceDir "node_modules\playwright"))) {
  Write-Host "[browser-use] 首次运行，正在安装依赖..."
  npm install --no-fund --no-audit
  if ($LASTEXITCODE -ne 0) {
    Pop-Location
    throw "依赖安装失败"
  }
}

Write-Host "[browser-use] 正在检查 Chromium 浏览器环境..."
npx playwright install chromium | Out-Null

Write-Host "[browser-use] 启动服务中..."
Start-Process powershell -ArgumentList '-NoExit', '-Command', "Set-Location '$serviceDir'; npm start"
Write-Host "[browser-use] 服务启动命令已发送。"
Write-Host "[browser-use] 访问健康检查：http://127.0.0.1:38765/health"

Pop-Location
