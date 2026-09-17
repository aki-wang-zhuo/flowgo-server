/**
 * 用 UTF-8 调用 FlowGo MCP：新建流程并通知编辑器打开。
 * 避免 Windows PowerShell 默认编码把中文变成 "?"。
 *
 * 用法: node scripts/mcp-save-utf8.mjs [流程名称]
 */
import http from 'node:http'

const BASE = { hostname: '127.0.0.1', port: 8090 }
const flowName = process.argv[2] || '编码测试MCP'

/**
 * 发送 HTTP 请求，请求/响应均按 UTF-8 处理。
 * @param {string} method
 * @param {string} path
 * @param {Record<string, string>} headers
 * @param {string | null} body
 */
function request(method, path, headers, body) {
  return new Promise((resolve, reject) => {
    const data = body != null ? Buffer.from(body, 'utf8') : null
    const req = http.request(
      {
        ...BASE,
        path,
        method,
        headers: {
          ...headers,
          ...(data ? { 'Content-Length': String(data.length) } : {}),
        },
      },
      (res) => {
        const chunks = []
        res.on('data', (c) => chunks.push(c))
        res.on('end', () => {
          resolve({
            status: res.statusCode,
            headers: res.headers,
            body: Buffer.concat(chunks).toString('utf8'),
          })
        })
      },
    )
    req.on('error', reject)
    if (data) req.write(data)
    req.end()
  })
}

async function main() {
  const login = JSON.parse(
    (
      await request(
        'POST',
        '/api/auth/login',
        { 'Content-Type': 'application/json; charset=utf-8' },
        JSON.stringify({ username: 'admin', password: 'admin' }),
      )
    ).body,
  )
  const token = login.token
  if (!token) throw new Error('login failed: ' + JSON.stringify(login))

  /** @type {Record<string, string>} */
  const auth = {
    Authorization: `Bearer ${token}`,
    'Content-Type': 'application/json; charset=utf-8',
    Accept: 'application/json, text/event-stream',
  }

  const init = await request(
    'POST',
    '/mcp',
    auth,
    JSON.stringify({
      jsonrpc: '2.0',
      id: 1,
      method: 'initialize',
      params: {
        protocolVersion: '2024-11-05',
        capabilities: {},
        clientInfo: { name: 'mcp-save-utf8', version: '1.0' },
      },
    }),
  )
  const sid = init.headers['mcp-session-id']
  if (!sid) throw new Error('no mcp session: ' + init.body)
  auth['Mcp-Session-Id'] = sid

  await request(
    'POST',
    '/mcp',
    auth,
    JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }),
  )

  const id = `enc-mcp-${Date.now()}`
  const dsl = JSON.stringify({
    id,
    name: flowName,
    entryNode: 'n1',
    nodes: [
      {
        id: 'n1',
        type: 'jsTransform',
        name: '转换',
        x: 120,
        y: 100,
        configuration: {
          jsScript:
            "msg.from='mcp-utf8'; return {'msg':msg,'metadata':metadata,'msgType':msgType,'dataType':dataType};",
        },
      },
    ],
    edges: [],
  })

  const save = await request(
    'POST',
    '/mcp',
    auth,
    JSON.stringify({
      jsonrpc: '2.0',
      id: 2,
      method: 'tools/call',
      params: { name: 'save_flow', arguments: { dsl } },
    }),
  )
  console.log('SAVE', save.body)

  const open = await request(
    'POST',
    '/mcp',
    auth,
    JSON.stringify({
      jsonrpc: '2.0',
      id: 3,
      method: 'tools/call',
      params: {
        name: 'notify_editor',
        arguments: { action: 'open_flow', flowId: id },
      },
    }),
  )
  console.log('OPEN', open.body)

  const get = await request('GET', `/api/flows/${id}`, {
    Authorization: `Bearer ${token}`,
  })
  console.log('GET', get.body)

  const rec = JSON.parse(get.body)
  if (rec.name !== flowName) {
    console.error('ENCODING FAIL: expected', flowName, 'got', rec.name)
    process.exit(1)
  }
  console.log('OK utf8 name preserved:', rec.name, 'id=', id)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
