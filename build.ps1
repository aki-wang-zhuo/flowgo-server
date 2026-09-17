# 编译 FlowGo Server 到本目录并可选启动。
#
# 用法：
#   .\build.ps1              # Windows amd64 → flowgo-server.exe，编译后启动
#   .\build.ps1 -linux       # Linux amd64，不启动
#   .\build.ps1 -NoStart     # 仅编译
param(
    [switch]$linux,
    [switch]$NoStart
)

$ErrorActionPreference = "Stop"

$ServerDir = $PSScriptRoot
$SrcMain = Join-Path $ServerDir "cmd\server\main.go"
if (-not (Test-Path $SrcMain)) {
    throw "未找到源码: $SrcMain"
}

if ($linux) {
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $outName = "flowgo-server"
} else {
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $outName = "flowgo-server.exe"
}
$outPath = Join-Path $ServerDir $outName

function Stop-FlowGoIfRunning {
    $procs = Get-Process -Name "flowgo-server" -ErrorAction SilentlyContinue
    if (-not $procs) { return }
    foreach ($p in $procs) {
        Write-Host "结束占用中的进程 PID=$($p.Id) ..."
        Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    }
    Start-Sleep -Milliseconds 500
}

if (-not $linux) {
    Stop-FlowGoIfRunning
}

Push-Location $ServerDir
try {
    Write-Host "go mod tidy ..."
    go mod tidy
    Write-Host "building $outName ..."
    go build -o $outPath .\cmd\server
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    Write-Host "built: $outPath"
} finally {
    Pop-Location
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
}

if ($linux -or $NoStart) {
    return
}

# 在当前终端前台启动，避免 Start-Process 另开命令行窗口
Write-Host "starting $outName ..."
Push-Location $ServerDir
try {
    & $outPath
} finally {
    Pop-Location
}
