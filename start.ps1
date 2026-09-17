# 启动已编译的 flowgo-server.exe（需先 .\build.ps1 -NoStart 或完整 build）
$ErrorActionPreference = "Stop"
$ServerDir = $PSScriptRoot
$exe = Join-Path $ServerDir "flowgo-server.exe"
if (-not (Test-Path $exe)) {
    throw "未找到 $exe，请先运行 .\build.ps1"
}
Start-Process -FilePath $exe -WorkingDirectory $ServerDir
Write-Host "已启动 flowgo-server（默认 :8090）"
