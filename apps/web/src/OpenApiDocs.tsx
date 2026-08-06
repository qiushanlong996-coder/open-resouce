import { useEffect, useRef, useState } from 'react'
import { ArrowRight, Check, Copy, KeyRound, ShieldCheck, Terminal, Users, X, Zap } from 'lucide-react'
import './open-api-docs.css'

function CodeBlock({ label, code }: { label: string; code: string }) {
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current)
  }, [])

  const onCopy = () => {
    void navigator.clipboard?.writeText(code).then(() => {
      setCopied(true)
      if (timer.current) clearTimeout(timer.current)
      timer.current = setTimeout(() => setCopied(false), 1600)
    })
  }

  return (
    <div className="oad-code">
      <div className="oad-code-head">
        <span>{label}</span>
        <button type="button" className="oad-copy" onClick={onCopy} aria-label={copied ? '已复制' : '复制代码'}>
          {copied ? <Check size={13} /> : <Copy size={13} />} {copied ? '已复制' : '复制'}
        </button>
      </div>
      <pre><code>{code}</code></pre>
    </div>
  )
}

const ENDPOINTS: { method: string; path: string; purpose: string }[] = [
  { method: 'POST', path: '/api/v1/open/files/presign', purpose: '为一次 OSS 上传预签名' },
  { method: 'POST', path: '/api/v1/open/projects', purpose: '创建草稿项目' },
  { method: 'POST', path: '/api/v1/open/projects/{id}/submit', purpose: '提交草稿进入审核' },
  { method: 'POST', path: '/api/v1/open/projects/{id}/documents', purpose: '追加知识库文档' },
  { method: 'GET', path: '/api/v1/open/projects', purpose: '列出已发布项目' },
]

const FIELDS: { field: string; rule: string }[] = [
  { field: 'slug', rule: '^[a-z0-9]+(?:-[a-z0-9]+)*$，≤ 80，全平台唯一' },
  { field: 'name', rule: '2-120 字符' },
  { field: 'summary', rule: '10-300 字符' },
  { field: 'description', rule: '20-50000 字符（Markdown）' },
  { field: 'category', rule: '非空，≤ 80' },
  { field: 'tags / tech_stack', rule: '各 ≤ 10 项' },
  { field: 'license', rule: '非空，≤ 40' },
  { field: 'current_version', rule: '非空，≤ 40' },
  { field: 'repository_url', rule: '≤ 500' },
  { field: '*_object_key', rule: '为空，或你预签名过的 uploads/user-<owner-id>/... key' },
]

const PRESIGN_CURL = `curl -sS https://api.openresource.cn/api/v1/open/files/presign \\
  -H "Authorization: Bearer $ORK_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"filename":"parsed.zip","content_type":"application/zip","size":4096,"kind":"code"}'`

const PRESIGN_RESP = `{
  "data": {
    "object_key": "uploads/user-<id>/2026/08/<random>.zip",
    "method": "PUT",
    "url": "https://<bucket>.<endpoint>/uploads/...?<v4-signature>",
    "headers": { "Content-Type": "application/zip" },
    "expires_at": "2026-08-01T12:10:00Z"
  },
  "request_id": "..."
}`

const UPLOAD_CURL = `curl -sS -X PUT "$PRESIGNED_URL" \\
  -H "Content-Type: application/zip" \\
  --data-binary @parsed.zip`

const CREATE_CURL = `curl -sS https://api.openresource.cn/api/v1/open/projects \\
  -H "Authorization: Bearer $ORK_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "slug": "agent-demo",
    "name": "Agent Demo",
    "summary": "A project published programmatically via the Open API.",
    "description": "# Overview\\n\\nMarkdown body, at least 20 characters long ...",
    "category": "Coding Agent",
    "tags": ["Agent", "MCP"],
    "tech_stack": ["Go", "React"],
    "license": "MIT",
    "repository_url": "https://github.com/example/agent",
    "current_version": "0.1.0",
    "code_object_key": "uploads/user-<id>/2026/08/<random>.zip",
    "document_object_key": "",
    "cover_object_key": ""
  }'`

const SUBMIT_CURL = `curl -sS https://api.openresource.cn/api/v1/open/projects/$PROJECT_ID/submit \\
  -X POST -H "Authorization: Bearer $ORK_KEY"`

const DOC_CURL = `curl -sS https://api.openresource.cn/api/v1/open/projects/$PROJECT_ID/documents \\
  -H "Authorization: Bearer $ORK_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"slug":"getting-started","title":"Getting Started","markdown":"# Hi\\n..."}'`

const AUTH_HEADER = `Authorization: Bearer ork_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`

export default function OpenApiDocs({ onClose }: { onClose: () => void }) {
  const panelRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    panelRef.current?.focus()
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div className="oad-backdrop" role="presentation" onClick={onClose}>
      <section
        className="oad-sheet"
        role="dialog"
        aria-modal="true"
        aria-labelledby="oad-title"
        ref={panelRef}
        tabIndex={-1}
        onClick={(event) => event.stopPropagation()}
      >
        <header className="oad-header">
          <div>
            <span className="oad-eyebrow">OPEN API · 面向 Agent 的开放能力</span>
            <h2 id="oad-title">开放能力接口文档</h2>
            <p>让 AI Agent、MCP 工具与自动化脚本，通过开放接口把项目自动发布到新猿译码。</p>
          </div>
          <button type="button" className="oad-close" onClick={onClose} aria-label="关闭">
            <X size={18} />
          </button>
        </header>

        <div className="oad-body">
          <div className="oad-highlights">
            <div className="oad-highlight">
              <Zap size={18} />
              <strong>能做什么</strong>
              <p>预签名上传代码归档、创建草稿项目、追加知识库文档，并提交进入平台审核。</p>
            </div>
            <div className="oad-highlight">
              <Users size={18} />
              <strong>面向谁</strong>
              <p>AI Agent 技能、MCP Server、CI 流水线与命令行脚本等需要程序化发布的场景。</p>
            </div>
            <div className="oad-highlight">
              <ShieldCheck size={18} />
              <strong>审核机制</strong>
              <p>提交只会将项目置为待审核，永不自动发布，必须经管理员审核通过后才对外可见。</p>
            </div>
          </div>

          <section className="oad-section">
            <h3><KeyRound size={17} /> 鉴权</h3>
            <p>
              所有开放接口位于 <code>/api/v1/open/*</code>，使用 Bearer AccessKey 鉴权，并以该
              AccessKey 所属账号的身份执行，创建的项目会归属到该账号，并出现在其作者中心。
            </p>
            <div className="oad-callout">
              <strong>如何获取 AccessKey</strong>
              <p>
                登录后点击<b>右上角头像账号菜单 → “AccessKey 管理”</b>，创建一个新的
                AccessKey。明文密钥（形如 <code>ork_&lt;hex&gt;</code>）仅在创建时展示一次，请妥善保存；
                服务端只保存其 SHA-256 摘要。
              </p>
            </div>
            <p>随后在每个请求头中携带它：</p>
            <CodeBlock label="请求头" code={AUTH_HEADER} />
            <p className="oad-note">
              缺失或格式错误的请求头、未知或已吊销的密钥 → <code>401</code>；
              密钥所属账号被封禁时，写接口 → <code>403</code>。成功响应包裹为
              {' '}<code>{'{ "data": ..., "request_id": "..." }'}</code>，错误则为
              {' '}<code>{'{ "error": { "code", "message" }, "request_id" }'}</code>。
            </p>
          </section>

          <section className="oad-section">
            <h3><ArrowRight size={17} /> 发布流程</h3>
            <ol className="oad-flow">
              <li><span>1</span><div><strong>预签名</strong><small>presign 拿到一次性 PUT 地址</small></div></li>
              <li><span>2</span><div><strong>上传归档</strong><small>PUT 代码包到 OSS</small></div></li>
              <li><span>3</span><div><strong>创建草稿</strong><small>引用 object_key 建项目</small></div></li>
              <li><span>4</span><div><strong>提交审核</strong><small>进入 pending_review</small></div></li>
              <li><span>5</span><div><strong>管理员通过</strong><small>→ published 发布</small></div></li>
            </ol>
            <p className="oad-note">
              状态流转：<code>draft → pending_review → published</code>（或 <code>rejected</code>，可编辑后重新提交）。
              代码解析与打包在客户端完成，平台只接收归档文件（<code>.zip</code> / <code>.gz</code> / <code>.tgz</code> / <code>.tar</code>）并按 object key 引用。
            </p>
          </section>

          <section className="oad-section">
            <h3><Terminal size={17} /> 接口</h3>

            <div className="oad-endpoint">
              <div className="oad-endpoint-head"><span className="oad-method">POST</span><code>/api/v1/open/files/presign</code></div>
              <p>为每个文件请求一次预签名 PUT。<code>kind</code> 为 <code>image</code>（封面）、<code>document</code>（pdf/md/txt）或 <code>code</code>（zip/gz/tgz/tar）。大小上限：image ≤ 10 MiB、document ≤ 50 MiB、code ≤ 500 MiB。URL 10 分钟内有效且禁止覆盖；类型与扩展名不匹配 → <code>422</code>（<code>invalid_file</code>）。</p>
              <CodeBlock label="请求" code={PRESIGN_CURL} />
              <CodeBlock label="响应" code={PRESIGN_RESP} />
            </div>

            <div className="oad-endpoint">
              <div className="oad-endpoint-head"><span className="oad-method">PUT</span><code>&lt;presigned url&gt;</code></div>
              <p>将文件字节直接 PUT 到预签名 <code>url</code>，并回显返回的 <code>headers</code>。保存好 <code>object_key</code>，创建项目时引用。</p>
              <CodeBlock label="上传归档" code={UPLOAD_CURL} />
            </div>

            <div className="oad-endpoint">
              <div className="oad-endpoint-head"><span className="oad-method">POST</span><code>/api/v1/open/projects</code></div>
              <p>创建草稿。成功返回 <code>201</code>，项目 <code>status</code> 为 <code>draft</code>，<code>owner_id</code> 为密钥所属账号。重复 slug → <code>409</code>（<code>project_slug_exists</code>）；非本账号的 object key → <code>422</code>（<code>invalid_project_file</code>）。</p>
              <CodeBlock label="请求" code={CREATE_CURL} />
            </div>

            <div className="oad-endpoint">
              <div className="oad-endpoint-head"><span className="oad-method">POST</span><code>/api/v1/open/projects/{'{id}'}/submit</code></div>
              <p>返回 <code>200</code>，状态变为 <code>pending_review</code>。仅密钥所属账号可提交自己的项目（否则 <code>404</code>）。之后由管理员审核通过（→ <code>published</code>）或驳回（→ <code>rejected</code>，可编辑并重新提交）。</p>
              <CodeBlock label="请求" code={SUBMIT_CURL} />
            </div>

            <div className="oad-endpoint">
              <div className="oad-endpoint-head"><span className="oad-method">POST</span><code>/api/v1/open/projects/{'{id}'}/documents</code></div>
              <p>（可选）追加知识库文档。字段：<code>slug</code>（<code>^[a-z0-9-]+$</code>，≤ 160）、<code>title</code>（1-200）、<code>markdown</code>（≤ 200000），可选 <code>parent_id</code>。</p>
              <CodeBlock label="请求" code={DOC_CURL} />
            </div>
          </section>

          <section className="oad-section">
            <h3>创建草稿的字段约束</h3>
            <div className="oad-table-wrap">
              <table className="oad-table">
                <thead><tr><th>字段</th><th>规则</th></tr></thead>
                <tbody>
                  {FIELDS.map((row) => (
                    <tr key={row.field}><td><code>{row.field}</code></td><td>{row.rule}</td></tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p className="oad-note">非法输入 → <code>422</code>（<code>invalid_project</code>）。所有字符串字段会被 trim，<code>slug</code> 会被转为小写。</p>
          </section>

          <section className="oad-section">
            <h3>接口一览</h3>
            <div className="oad-table-wrap">
              <table className="oad-table">
                <thead><tr><th>方法</th><th>路径</th><th>用途</th></tr></thead>
                <tbody>
                  {ENDPOINTS.map((row) => (
                    <tr key={`${row.method} ${row.path}`}>
                      <td><span className="oad-method sm">{row.method}</span></td>
                      <td><code>{row.path}</code></td>
                      <td>{row.purpose}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </div>
      </section>
    </div>
  )
}
