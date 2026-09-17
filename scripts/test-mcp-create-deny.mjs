/**
 * 测试：关闭 flowCreate 后，MCP save_flow 新建应被拒绝；更新已有流程仍可成功（若 flowUpdate 开启）。
 */
import http from 'node:http'

const BASE = { hostname: '127.0.0.1', port: 8090 }

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
  const authHdr = { Authorization: `Bearer ${token}` }

  const settings = JSON.parse(
    (await request('GET', '/api/settings/mcp', authHdr)).body,
  )
  console.log('当前 MCP 权限:', JSON.stringify(settings.permissions))

  const mcpAuth = {
    ...authHdr,
    'Content-Type': 'application/json; charset=utf-8',
    Accept: 'application/json, text/event-stream',
  }
  const init = await request(
    'POST',
    '/mcp',
    mcpAuth,
    JSON.stringify({
      jsonrpc: '2.0',
      id: 1,
      method: 'initialize',
      params: {
        protocolVersion: '2024-11-05',
        capabilities: {},
        clientInfo: { name: 'perm-test', version: '1' },
      },
    }),
  )
  const sid = init.headers['mcp-session-id']
  mcpAuth['Mcp-Session-Id'] = sid
  await request(
    'POST',
    '/mcp',
    mcpAuth,
    JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }),
  )

  const newId = `deny-create-${Date.now()}`
  const dsl = JSON.stringify({
    id: newId,
    name: '应被拒绝的新建',
    entryNode: 'n1',
    nodes: [
      {
        id: 'n1',
        type: 'jsTransform',
        name: 't',
        x: 0,
        y: 0,
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
    mcpAuth,
    JSON.stringify({
      jsonrpc: '2.0',
      id: 2,
      method: 'tools/call',
      params: { name: 'save_flow', arguments: { dsl } },
    }),
  )
  console.log('新建 save_flow:', create.body)

  // 找一个已有流程做更新对照（若 flowUpdate 仍开）
  const list = JSON.parse(
    (
      await request(
        'POST',
        '/mcp',
        mcpAuth,
        JSON.stringify({
          jsonrpc: '2.0',
          id: 3,
          method: 'tools/call',
          params: { name: 'list_flows', arguments: {} },
        }),
      )
    ).body,
  )
  const text = list?.result?.content?.[0]?.text
  const flows = text ? JSON.parse(text) : []
  const existing = Array.isArray(flows) ? flows[0] : null
  if (existing?.id) {
    const full = JSON.parse(
      (await request('GET', `/api/flows/${existing.id}`, authHdr)).body,
    )
    const updDsl = JSON.stringify({
      ...full.dsl,
      name: full.dsl?.name || existing.name,
      description: `perm-test-update-${Date.now()}`,
    })
    const upd = await request(
      'POST',
      '/mcp',
      mcpAuth,
      JSON.stringify({
        jsonrpc: '2.0',
        id: 4,
        method: 'tools/call',
        params: { name: 'save_flow', arguments: { dsl: updDsl } },
      }),
    )
    console.log('更新已有流程:', upd.body)
  } else {
    console.log('无已有流程，跳过更新对照')
  }

  const createText = create.body || ''
  if (
    createText.includes('permission denied') ||
    createText.includes('flowCreate')
  ) {
    console.log('OK: 新建已被权限拒绝')
    process.exit(0)
  }
  console.error('FAIL: 新建未被拒绝')
  process.exit(1)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
