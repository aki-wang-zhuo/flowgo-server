/**
 * 测试：flowCreate 开启后 MCP 新建应成功。
 */
import http from 'node:http'

function request(method, path, headers, body) {
  return new Promise((resolve, reject) => {
    const data = body != null ? Buffer.from(body, 'utf8') : null
    const req = http.request(
      {
        hostname: '127.0.0.1',
        port: 8090,
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
        res.on('end', () =>
          resolve({
            status: res.statusCode,
            headers: res.headers,
            body: Buffer.concat(chunks).toString('utf8'),
          }),
        )
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
  const settings = JSON.parse(
    (await request('GET', '/api/settings/mcp', { Authorization: `Bearer ${token}` })).body,
  )
  console.log('权限:', JSON.stringify(settings.permissions))

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
        clientInfo: { name: 'allow-test', version: '1' },
      },
    }),
  )
  auth['Mcp-Session-Id'] = init.headers['mcp-session-id']
  await request(
    'POST',
    '/mcp',
    auth,
    JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }),
  )

  const id = `allow-create-${Date.now()}`
  const dsl = JSON.stringify({
    id,
    name: '权限开启后新建',
    entryNode: 'n1',
    nodes: [
      {
        id: 'n1',
        type: 'jsTransform',
        name: 't',
        x: 80,
        y: 80,
        configuration: {
          jsScript:
            "return {'msg':msg,'metadata':metadata,'msgType':msgType,'dataType':dataType};",
        },
      },
    ],
    edges: [],
  })
  const create = await request(
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
  console.log('新建:', create.body)

  await request(
    'POST',
    '/mcp',
    auth,
    JSON.stringify({
      jsonrpc: '2.0',
      id: 3,
      method: 'tools/call',
      params: { name: 'notify_editor', arguments: { action: 'open_flow', flowId: id } },
    }),
  )

  if (create.body.includes('permission denied')) {
    console.error('FAIL: 仍被拒绝（请确认设置里已打开「新建流程」并保存）')
    process.exit(1)
  }
  if (create.body.includes(id) && create.body.includes('权限开启后新建')) {
    console.log('OK: 新建成功 id=' + id)
    process.exit(0)
  }
  console.error('FAIL: 响应异常')
  process.exit(1)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
