#Requires -Version 5.1
<#
.SYNOPSIS
  以 UTF-8 字节调用 FlowGo MCP tools/call，避免 Windows PowerShell 默认编码把中文变成 "?"。

.DESCRIPTION
  Windows PowerShell 5.x 把 -Body 字符串按系统 ANSI 发出时，无法表示的字符会变成 "?"。
  本脚本统一用 [Text.Encoding]::UTF8.GetBytes 发送，并声明 charset=utf-8。

.EXAMPLE
  .\Invoke-McpTool.ps1 -Tool save_flow -Arguments @{ dsl = '{"id":"x","name":"中文","entryNode":"n1","nodes":[],"edges":[]}' }
#>
param(
  [Parameter(Mandatory = $true)]
  [string]$Tool,

  [hashtable]$Arguments = @{},

  [string]$BaseUrl = 'http://127.0.0.1:8090',

  [string]$Username = 'admin',

  [string]$Password = 'admin'
)

$ErrorActionPreference = 'Stop'
$utf8 = [System.Text.Encoding]::UTF8

function Send-Utf8Json {
  param(
    [string]$Method,
    [string]$Uri,
    [hashtable]$Headers,
    [string]$JsonBody
  )
  $bytes = $utf8.GetBytes($JsonBody)
  $hdrs = @{}
  foreach ($k in $Headers.Keys) { $hdrs[$k] = $Headers[$k] }
  if (-not $hdrs.ContainsKey('Content-Type')) {
    $hdrs['Content-Type'] = 'application/json; charset=utf-8'
  }
  return Invoke-WebRequest -Method $Method -Uri $Uri -Headers $hdrs -Body $bytes -UseBasicParsing
}

# 登录
$loginBody = (@{ username = $Username; password = $Password } | ConvertTo-Json -Compress)
$loginResp = Send-Utf8Json -Method Post -Uri "$BaseUrl/api/auth/login" -Headers @{} -JsonBody $loginBody
$login = $loginResp.Content | ConvertFrom-Json
if (-not $login.token) { throw "login failed: $($loginResp.Content)" }
$token = [string]$login.token

$mcpHeaders = @{
  Authorization     = "Bearer $token"
  Accept            = 'application/json, text/event-stream'
  'Content-Type'    = 'application/json; charset=utf-8'
}

# initialize
$initBody = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"Invoke-McpTool","version":"1.0"}}}'
$init = Send-Utf8Json -Method Post -Uri "$BaseUrl/mcp" -Headers $mcpHeaders -JsonBody $initBody
$sid = $init.Headers['Mcp-Session-Id']
if (-not $sid) { $sid = $init.Headers['mcp-session-id'] }
if (-not $sid) { throw "no Mcp-Session-Id: $($init.Content)" }
$mcpHeaders['Mcp-Session-Id'] = [string]$sid

Send-Utf8Json -Method Post -Uri "$BaseUrl/mcp" -Headers $mcpHeaders -JsonBody '{"jsonrpc":"2.0","method":"notifications/initialized"}' | Out-Null

# 用 .NET JavaScriptSerializer / 手工拼装避免 ConvertTo-Json 转义问题：这里用 Newtonsoft 不可用，改用嵌套 hashtable + 深度序列化后强制 UTF-8
# PowerShell ConvertTo-Json 对 unicode 本身是安全的（字符串在内存中是 UTF-16），关键在发出 HTTP 时。
$rpc = @{
  jsonrpc = '2.0'
  id      = 2
  method  = 'tools/call'
  params  = @{
    name      = $Tool
    arguments = $Arguments
  }
}
# Depth 要够深；-Compress 减少体积
$rpcJson = $rpc | ConvertTo-Json -Depth 20 -Compress
$call = Send-Utf8Json -Method Post -Uri "$BaseUrl/mcp" -Headers $mcpHeaders -JsonBody $rpcJson

# 按 UTF-8 输出，避免控制台二次乱码
$out = $utf8.GetString($utf8.GetBytes($call.Content))
Write-Output $call.Content
