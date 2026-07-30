$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot

python -m pip install -r requirements.txt
python -m pip install pyinstaller

python -m PyInstaller `
  --noconfirm `
  --clean `
  --windowed `
  --name synapse-executor `
  --icon "assets\executor-icon.ico" `
  --add-data "assets\executor-icon.ico;assets" `
  --add-data "assets\executor-icon.png;assets" `
  --paths . `
  --hidden-import app.api.routes `
  --hidden-import app.services.heartbeat_client `
  --hidden-import app.runners.playwright_runner `
  --hidden-import app.runners.pytest_runner `
  --hidden-import uvicorn.logging `
  --hidden-import uvicorn.loops.auto `
  --hidden-import uvicorn.protocols.http.auto `
  --hidden-import uvicorn.protocols.websockets.auto `
  gui.py

Write-Host "执行器 exe 已生成：$PSScriptRoot\dist\synapse-executor\synapse-executor.exe"
