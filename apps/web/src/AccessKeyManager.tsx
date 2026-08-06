import { useCallback, useEffect, useState } from 'react'
import { X, KeyRound, Copy, Check, RefreshCw } from 'lucide-react'
import { getMyApiKeys, createMyApiKey, revokeMyApiKey, type ApiKey } from './api/client'
import './access-key.css'

const errorMessage = (value: unknown, fallback: string) =>
  value instanceof Error ? value.message : fallback

function formatDate(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}

export default function AccessKeyManager({ onClose }: { onClose: () => void }) {
  const [keys, setKeys] = useState<ApiKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [creating, setCreating] = useState(false)
  const [issued, setIssued] = useState('')
  const [copied, setCopied] = useState(false)
  const [busy, setBusy] = useState<string | null>(null)
  const [confirming, setConfirming] = useState<string | null>(null)

  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true)
    setError('')
    getMyApiKeys(signal)
      .then((response) => setKeys(response.data))
      .catch((value) => { if (!signal?.aborted) setError(errorMessage(value, 'AccessKey 加载失败')) })
      .finally(() => { if (!signal?.aborted) setLoading(false) })
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    load(controller.signal)
    return () => controller.abort()
  }, [load])

  const doCreate = async () => {
    if (!name.trim()) { setError('请填写 AccessKey 名称'); return }
    setCreating(true)
    setError('')
    try {
      const response = await createMyApiKey(name.trim())
      setIssued(response.data.plaintext)
      setCopied(false)
      setName('')
      load()
    } catch (value) {
      setError(errorMessage(value, '创建失败'))
    } finally {
      setCreating(false)
    }
  }

  const doCopy = () => {
    void navigator.clipboard?.writeText(issued).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    })
  }

  const doRevoke = async (key: ApiKey) => {
    setBusy(key.id)
    setError('')
    try {
      await revokeMyApiKey(key.id)
      setConfirming(null)
      load()
    } catch (value) {
      setError(errorMessage(value, '撤销失败'))
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="ak-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="ak-shell"
        role="dialog"
        aria-modal="true"
        aria-label="AccessKey 管理"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="ak-header">
          <div>
            <h2>AccessKey 管理</h2>
            <p>用于 Open API 的访问密钥。以 <code className="ak-mono">Authorization: Bearer &lt;key&gt;</code> 调用 <code className="ak-mono">/api/v1/open/*</code>，将以你的身份发布项目与文档。</p>
          </div>
          <button className="ak-close" onClick={onClose} aria-label="关闭"><X size={20} /></button>
        </header>
        <div className="ak-body">
          {issued && (
            <div className="ak-notice">
              <span className="ak-warning">只显示一次，请立即保存</span>
              <span className="ak-notice-key">{issued}</span>
              <button className="ak-btn" onClick={doCopy}>
                {copied ? <><Check size={14} /> 已复制</> : <><Copy size={14} /> 复制</>}
              </button>
              <button className="ak-btn" onClick={() => setIssued('')}>我已保存</button>
            </div>
          )}

          <div className="ak-form">
            <input
              placeholder="新 AccessKey 名称，如「本地 Agent」"
              value={name}
              maxLength={120}
              onChange={(event) => setName(event.target.value)}
              onKeyDown={(event) => { if (event.key === 'Enter' && !creating) void doCreate() }}
            />
            <button className="ak-btn primary" disabled={creating} onClick={() => void doCreate()}>
              <KeyRound size={15} /> {creating ? '创建中…' : '创建 AccessKey'}
            </button>
            <button className="ak-btn" onClick={() => load()} aria-label="刷新"><RefreshCw size={15} /></button>
          </div>

          <p className="ak-hint">
            这些密钥用于 Open API 的 Bearer 鉴权（<code>/api/v1/open/*</code>），供外部 AI Agent / MCP 工具以你的身份发布内容。用法详见项目文档 <code>docs/open-api.md</code>。密钥明文仅在创建时展示一次，服务端只保存其摘要。
          </p>

          {error && <div className="ak-error">{error}</div>}

          {loading ? <div className="ak-state">正在加载…</div>
            : keys.length === 0 ? <div className="ak-state">你还没有创建任何 AccessKey。</div>
              : (
                <div className="ak-list">
                  {keys.map((key) => (
                    <div className="ak-row" key={key.id}>
                      <div className="ak-row-main">
                        <div className="ak-row-name">{key.name}</div>
                        <div className="ak-row-meta">
                          <span className="ak-mono">{key.prefix}…</span> · 创建于 {formatDate(key.created_at)}
                          {key.revoked_at ? ` · 撤销于 ${formatDate(key.revoked_at)}` : ''}
                        </div>
                      </div>
                      {key.revoked_at
                        ? <span className="ak-badge revoked">已撤销</span>
                        : confirming === key.id
                          ? (
                            <span className="ak-confirm">
                              确认撤销？
                              <button className="ak-btn danger" disabled={busy === key.id} onClick={() => void doRevoke(key)}>
                                {busy === key.id ? '撤销中…' : '确认'}
                              </button>
                              <button className="ak-btn" onClick={() => setConfirming(null)}>取消</button>
                            </span>
                          )
                          : (
                            <>
                              <span className="ak-badge ok">有效</span>
                              <button className="ak-btn danger" onClick={() => setConfirming(key.id)}>撤销</button>
                            </>
                          )}
                    </div>
                  ))}
                </div>
              )}
        </div>
      </div>
    </div>
  )
}
