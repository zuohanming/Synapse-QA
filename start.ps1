$ErrorActionPreference = "Stop"

if (-not $env:PGPASSWORD) {
  $env:PGPASSWORD = Read-Host "请输入 PostgreSQL postgres 用户密码"
}
$dbName = "synapse_qa"

$dbExists = psql -h 127.0.0.1 -U postgres -d postgres -tAc "select 1 from pg_database where datname='$dbName'" 2>$null
if (($dbExists | Out-String).Trim() -ne "1") {
  createdb -h 127.0.0.1 -U postgres $dbName
}

$apiPort = 8080
$apiConn = Get-NetTCPConnection -LocalPort $apiPort -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $apiConn) {
  Start-Process -FilePath powershell -ArgumentList "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "$PSScriptRoot\backend\start-api.ps1" -WindowStyle Hidden
}

$webPort = 4173
$webConn = Get-NetTCPConnection -LocalPort $webPort -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $webConn) {
  $frontendDir = Join-Path $PSScriptRoot "frontend"
  if (-not (Test-Path (Join-Path $frontendDir "node_modules"))) {
    Push-Location $frontendDir
    npm install
    Pop-Location
  }
  Start-Process -FilePath "npm.cmd" -ArgumentList "run", "dev", "--", "--host", "127.0.0.1", "--port", "$webPort" -WorkingDirectory $frontendDir -WindowStyle Hidden
}

$executorPort = 8090
$executorConn = Get-NetTCPConnection -LocalPort $executorPort -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $executorConn) {
  $executorDir = Join-Path $PSScriptRoot "executor"
  $venvPython = Join-Path $executorDir ".venv\Scripts\python.exe"
  if (-not (Test-Path $venvPython)) {
    python -m venv (Join-Path $executorDir ".venv")
  }
  & $venvPython -m pip install -r (Join-Path $executorDir "requirements.txt")
  Start-Process -FilePath $venvPython -ArgumentList "main.py" -WorkingDirectory $executorDir -WindowStyle Hidden
}

Start-Sleep -Seconds 5

$apiHealth = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$apiPort/api/health" -TimeoutSec 5
$webHealth = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$webPort/" -TimeoutSec 5
$executorHealth = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$executorPort/health" -TimeoutSec 5

Write-Host "API: http://127.0.0.1:$apiPort ($($apiHealth.StatusCode))"
Write-Host "Web: http://127.0.0.1:$webPort ($($webHealth.StatusCode))"
Write-Host "Executor: http://127.0.0.1:$executorPort ($($executorHealth.StatusCode))"
Write-Host "默认账号：admin / admin123"
