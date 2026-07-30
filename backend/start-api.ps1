$localEnvPath = Join-Path $PSScriptRoot ".env.local"
if (Test-Path -LiteralPath $localEnvPath) {
  foreach ($line in Get-Content -LiteralPath $localEnvPath -Encoding UTF8) {
    $trimmed = $line.Trim()
    if (-not $trimmed -or $trimmed.StartsWith("#") -or -not $trimmed.Contains("=")) {
      continue
    }
    $name, $value = $trimmed.Split("=", 2)
    [Environment]::SetEnvironmentVariable($name, $value, "Process")
  }
}

if (-not $env:DATABASE_URL) {
  if (-not $env:PGPASSWORD) {
    $env:PGPASSWORD = Read-Host "请输入 PostgreSQL postgres 用户密码"
  }
  $env:DATABASE_URL = "postgres://postgres:$($env:PGPASSWORD)@127.0.0.1:5432/synapse_qa?sslmode=disable"
}
if (-not $env:API_ADDR) {
  $env:API_ADDR = "127.0.0.1:8080"
}
Set-Location $PSScriptRoot
go build -o synapse-api.exe ./cmd/api
& "$PSScriptRoot\synapse-api.exe"
