#!/usr/bin/env node
// 协作编辑双端同步的协议级验证。
//
// 为什么需要这个脚本：
//   - Go 集成测试只验证服务端把消息转发给了同房间的其他连接，
//     并不能证明两个 Yjs 文档真的收敛到相同内容。
//   - 浏览器自动化工具开不了第二个标签页，无法做真正的双客户端 UI 验证。
//
// 这里用两个真实的 Y.Doc + 真实 WebSocket 连接，走与前端完全相同的线协议
// （init / update / awareness / presence / snapshot，二进制经 Base64），
// 验证 CRDT 是否收敛，以及不同文档是否互不串内容。
//
// 用法：
//   COLLAB_BASE_URL=https://127.0.0.1:8443 \
//   COLLAB_COOKIE='or_session=...' \
//   node scripts/collab-two-client-check.mjs <project-slug> <doc-a> <doc-b>

import { WebSocket } from 'ws'
import * as Y from 'yjs'

const baseURL = process.env.COLLAB_BASE_URL
const cookie = process.env.COLLAB_COOKIE
const [projectSlug, documentA, documentB] = process.argv.slice(2)

if (!baseURL || !cookie || !projectSlug || !documentA) {
  console.error('用法: COLLAB_BASE_URL=... COLLAB_COOKIE=... node collab-two-client-check.mjs <project-slug> <doc-a> [doc-b]')
  process.exit(2)
}

const bytesToBase64 = (bytes) => Buffer.from(bytes).toString('base64')
const base64ToBytes = (value) => new Uint8Array(Buffer.from(value, 'base64'))
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

let failures = 0
function report(label, ok, detail = '') {
  console.log(`${ok ? 'PASS' : 'FAIL'} ${label}${detail ? `: ${detail}` : ''}`)
  if (!ok) failures += 1
}

// connect 建立一个协作客户端，行为与前端 CollaborativeMarkdownEditor 一致。
function connect(name, documentSlug) {
  return new Promise((resolve, reject) => {
    const url = new URL(`${baseURL}/api/v1/projects/${encodeURIComponent(projectSlug)}/collaboration/ws`)
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
    if (documentSlug) url.search = `?document=${encodeURIComponent(documentSlug)}`

    const socket = new WebSocket(url, {
      headers: { Cookie: cookie },
      // 生产用自签名证书，这里跳过校验；仅用于本地验证脚本。
      rejectUnauthorized: false,
    })
    const yDoc = new Y.Doc()
    const remoteOrigin = Symbol('remote')
    const client = { name, socket, yDoc, initial: null, saved: [], errors: [] }

    const send = (payload) => {
      if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify(payload))
    }
    client.send = send

    socket.on('error', reject)
    socket.on('message', (raw) => {
      const message = JSON.parse(raw.toString())
      switch (message.type) {
        case 'init':
          client.initial = message
          if (message.snapshot) Y.applyUpdate(yDoc, base64ToBytes(message.snapshot), remoteOrigin)
          // 本地更新回传服务端，与前端一致。
          yDoc.on('update', (update, origin) => {
            if (origin !== remoteOrigin) send({ type: 'update', update: bytesToBase64(update) })
          })
          send({ type: 'presence', client_id: yDoc.clientID })
          resolve(client)
          break
        case 'update':
          Y.applyUpdate(yDoc, base64ToBytes(message.update), remoteOrigin)
          break
        case 'saved':
          client.saved.push(message)
          break
        case 'error':
          client.errors.push(message.message)
          break
        default:
          break
      }
    })
    socket.on('close', (code, reason) => {
      client.closed = { code, reason: reason.toString() }
    })
  })
}

const textOf = (client) => client.yDoc.getText('markdown').toString()

async function main() {
  console.log('=== 1. 两个客户端连接同一篇文档 ===')
  const a = await connect('A', documentA)
  const b = await connect('B', documentA)
  report('client_a_init', a.initial?.type === 'init', `markdown 长度 ${a.initial?.markdown?.length ?? 0}`)
  report('client_b_init', b.initial?.type === 'init', `markdown 长度 ${b.initial?.markdown?.length ?? 0}`)
  report('init_markdown_same', a.initial?.markdown === b.initial?.markdown)

  console.log('=== 2. A 输入，B 应收到（正向同步）===')
  a.yDoc.getText('markdown').insert(0, 'AAA-from-a-')
  await sleep(1200)
  report('b_received_a', textOf(b).includes('AAA-from-a-'), `B 内容="${textOf(b)}"`)

  console.log('=== 3. B 输入，A 应收到（反向同步）===')
  b.yDoc.getText('markdown').insert(textOf(b).length, '-BBB-from-b')
  await sleep(1200)
  report('a_received_b', textOf(a).includes('-BBB-from-b'), `A 内容="${textOf(a)}"`)

  console.log('=== 4. 双端收敛到同一内容（CRDT 核心保证）===')
  const convergedA = textOf(a)
  const convergedB = textOf(b)
  report('converged', convergedA === convergedB, `A="${convergedA}" B="${convergedB}"`)
  report('both_edits_survive',
    convergedA.includes('AAA-from-a-') && convergedA.includes('-BBB-from-b'),
    '两端编辑都保留，未互相覆盖')

  console.log('=== 5. 并发同时输入不丢内容 ===')
  a.yDoc.getText('markdown').insert(0, 'CONCURRENT-A|')
  b.yDoc.getText('markdown').insert(0, 'CONCURRENT-B|')
  await sleep(1500)
  const finalA = textOf(a)
  const finalB = textOf(b)
  report('concurrent_converged', finalA === finalB, `A="${finalA}"`)
  report('concurrent_both_kept',
    finalA.includes('CONCURRENT-A|') && finalA.includes('CONCURRENT-B|'),
    '并发插入双方内容都在')

  console.log('=== 6. 保存后另一端收到 saved 广播 ===')
  const markdown = `# 双端验证\n\n${finalA}\n\n这段由双端同步脚本写入。`
  a.send({ type: 'snapshot', snapshot: bytesToBase64(Y.encodeStateAsUpdate(a.yDoc)), markdown })
  await sleep(1500)
  report('a_got_saved_ack', a.saved.length > 0, `revision=${a.saved.at(-1)?.revision}`)
  report('b_got_saved_broadcast', b.saved.length > 0, `revision=${b.saved.at(-1)?.revision}`)

  if (documentB) {
    console.log('=== 7. 另一篇文档独立房间，不串内容 ===')
    const c = await connect('C', documentB)
    report('doc_b_init_differs', c.initial?.markdown !== a.initial?.markdown,
      `文档乙初始内容与甲不同`)
    const beforeC = textOf(c)
    a.yDoc.getText('markdown').insert(0, 'LEAK-CHECK|')
    await sleep(1500)
    report('no_cross_document_leak', !textOf(c).includes('LEAK-CHECK|'),
      `文档乙内容="${textOf(c)}"`)
    report('doc_b_unchanged', textOf(c) === beforeC, '文档乙未被文档甲的编辑影响')
    c.socket.close()
  }

  console.log('=== 8. 无协议错误 ===')
  report('no_errors_a', a.errors.length === 0, a.errors.join('; ') || '无')
  report('no_errors_b', b.errors.length === 0, b.errors.join('; ') || '无')

  a.socket.close()
  b.socket.close()
  await sleep(300)

  console.log(failures === 0 ? 'RESULT=ALL_PASS' : `RESULT=HAS_FAILURE(${failures})`)
  process.exit(failures === 0 ? 0 : 1)
}

main().catch((error) => {
  console.error('脚本异常:', error)
  process.exit(1)
})
