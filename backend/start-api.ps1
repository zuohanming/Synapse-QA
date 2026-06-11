if (-not $env:DATABASE_URL) {
  if (-not $env:PGPASSWORD) {
    $env:PGPASSWORD = Read-Host "请输入 PostgreSQL postgres 用户密码"
  }
  $env:DATABASE_URL = "postgres://postgres:$($env:PGPASSWORD)@127.0.0.1:5432/synapse_qa?sslmode=disable"
}
$env:API_ADDR = "127.0.0.1:8080"
Set-Location $PSScriptRoot
go build -o synapse-api.exe ./cmd/api
& "$PSScriptRoot\synapse-api.exe"
