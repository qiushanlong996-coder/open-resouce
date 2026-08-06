import { useEffect, useMemo, useState } from 'react'
import { History, RotateCcw, X } from 'lucide-react'
import {
  ApiError,
  getAuthorDocumentRevision,
  getAuthorDocumentRevisions,
  restoreAuthorDocumentRevision,
  type DocumentRevisionSource,
  type DocumentRevisionSummary,
  type ProjectDocument,
} from './api/client'
import { collapseDiffRows, diffLines } from './diffLines'
import type { DiffResult } from './diffLines'
import './document-revisions.css'

const SOURCE_LABEL: Record<DocumentRevisionSource, string> = {
  create: '创建',
  edit: '编辑',
  restore: '回滚',
}

// formatRevisionTime 用中文本地时间显示版本时间，精确到分钟。
function formatRevisionTime(value: string) {
  const time = new Date(value)
  if (Number.isNaN(time.getTime())) return value
  return time.toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  })
}

// formatCharDelta 展示相对上一个版本的体量变化，供快速判断哪次改动最大。
function formatCharDelta(current: number, previous: number | undefined) {
  if (previous === undefined) return `${current} 字符`
  const delta = current - previous
  if (delta === 0) return `${current} 字符 · 体量未变`
  return `${current} 字符 · ${delta > 0 ? '+' : '−'}${Math.abs(delta)}`
}

// DocumentRevisionPanel 文章历史版本面板：列出版本、预览正文、与当前正文逐行对比、回滚。
//
// 对比的「新」一侧固定是编辑器里的当前正文（含未保存改动），
// 这样作者在回滚前能确切看到自己会失去什么。
export default function DocumentRevisionPanel({
  projectID,
  document,
  currentMarkdown,
  onClose,
  onRestored,
  onNotify,
}: {
  projectID: string
  document: ProjectDocument
  currentMarkdown: string
  onClose: () => void
  onRestored: (restored: ProjectDocument) => void
  onNotify: (message: string) => void
}) {
  const [revisions, setRevisions] = useState<DocumentRevisionSummary[]>([])
  const [currentVersion, setCurrentVersion] = useState(document.version)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null)
  const [selectedMarkdown, setSelectedMarkdown] = useState<string | null>(null)
  const [contentLoading, setContentLoading] = useState(false)
  const [mode, setMode] = useState<'diff' | 'source'>('diff')
  const [restoring, setRestoring] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    getAuthorDocumentRevisions(projectID, document.id, controller.signal)
      .then((response) => {
        setRevisions(response.data)
        setCurrentVersion(response.current_version)
        // 默认选中比当前版本更早的那一条，这是「想撤回刚才的改动」最常见的诉求。
        const fallback = response.data.find((entry) => !entry.current) ?? response.data[0]
        setSelectedVersion(fallback ? fallback.version : null)
        setError('')
      })
      .catch((reason: unknown) => {
        if (reason instanceof DOMException && reason.name === 'AbortError') return
        setError(reason instanceof Error ? reason.message : '历史版本加载失败')
      })
      .finally(() => setLoading(false))
    return () => controller.abort()
  }, [projectID, document.id])

  useEffect(() => {
    if (selectedVersion === null) {
      setSelectedMarkdown(null)
      return
    }
    const controller = new AbortController()
    setContentLoading(true)
    getAuthorDocumentRevision(projectID, document.id, selectedVersion, controller.signal)
      .then((response) => {
        setSelectedMarkdown(response.data.markdown ?? '')
        setError('')
      })
      .catch((reason: unknown) => {
        if (reason instanceof DOMException && reason.name === 'AbortError') return
        setSelectedMarkdown(null)
        setError(reason instanceof Error ? reason.message : '版本内容加载失败')
      })
      .finally(() => setContentLoading(false))
    return () => controller.abort()
  }, [projectID, document.id, selectedVersion])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  const diff = useMemo<DiffResult | null>(() => {
    if (selectedMarkdown === null) return null
    return diffLines(selectedMarkdown, currentMarkdown)
  }, [selectedMarkdown, currentMarkdown])

  const chunks = useMemo(() => (diff ? collapseDiffRows(diff.rows) : []), [diff])

  const selected = revisions.find((entry) => entry.version === selectedVersion) ?? null

  const restore = async () => {
    if (selectedVersion === null || restoring) return
    setRestoring(true)
    try {
      const response = await restoreAuthorDocumentRevision(projectID, document.id, selectedVersion)
      onRestored(response.data)
      onNotify(`已回滚到 v${selectedVersion}，并存为 v${response.data.version}`)
      onClose()
    } catch (reason) {
      setError(reason instanceof ApiError ? reason.message : '回滚失败，请稍后重试')
      setRestoring(false)
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="revision-panel"
        role="dialog"
        aria-modal="true"
        aria-label={`《${document.title}》历史版本`}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <button className="icon-button modal-close" onClick={onClose} aria-label="关闭"><X size={17} /></button>
        <header className="revision-head">
          <div>
            <span className="section-kicker">ARTICLE / HISTORY</span>
            <h2><History size={18} /> 《{document.title}》历史版本</h2>
            <p>连续自动保存会合并为同一个版本，列表里的每一条都是可用的还原点。当前正文为 v{currentVersion}。</p>
          </div>
        </header>

        <div className="revision-body">
          <nav className="revision-list" aria-label="历史版本列表">
            {loading ? (
              <p className="revision-empty">正在加载历史版本…</p>
            ) : revisions.length === 0 ? (
              <p className="revision-empty">这篇文章还没有历史版本。保存一次正文后就会出现。</p>
            ) : (
              revisions.map((entry, index) => (
                <button
                  key={entry.id}
                  type="button"
                  className={`revision-row ${entry.version === selectedVersion ? 'is-active' : ''} ${entry.current ? 'is-current' : ''}`}
                  aria-current={entry.version === selectedVersion ? 'true' : undefined}
                  onClick={() => setSelectedVersion(entry.version)}
                >
                  <span className="revision-row-top">
                    <strong>v{entry.version}</strong>
                    <span className={`revision-source is-${entry.source}`}>
                      {SOURCE_LABEL[entry.source]}
                      {entry.source === 'restore' && entry.restored_from ? ` 自 v${entry.restored_from}` : ''}
                    </span>
                    {entry.current && <span className="revision-current-tag">当前</span>}
                  </span>
                  <span className="revision-row-meta">
                    {entry.author_name || '未知作者'} · {formatRevisionTime(entry.updated_at)}
                  </span>
                  <span className="revision-row-size">
                    {formatCharDelta(entry.char_count, revisions[index + 1]?.char_count)}
                  </span>
                </button>
              ))
            )}
          </nav>

          <section className="revision-detail" aria-label="版本内容">
            <div className="revision-detail-head">
              <div className="revision-mode-switch" role="group" aria-label="查看方式">
                <button type="button" className={mode === 'diff' ? 'active' : ''} onClick={() => setMode('diff')}>
                  与当前正文对比
                </button>
                <button type="button" className={mode === 'source' ? 'active' : ''} onClick={() => setMode('source')}>
                  该版本原文
                </button>
              </div>
              {diff && mode === 'diff' && (
                <span className="revision-diff-stat">
                  <span className="stat-added">+{diff.added}</span>
                  <span className="stat-removed">−{diff.removed}</span>
                  {diff.truncated && <span className="stat-note">文档过长，已按整块替换展示</span>}
                </span>
              )}
              <button
                type="button"
                className="revision-restore"
                disabled={selectedVersion === null || contentLoading || restoring || Boolean(selected?.current)}
                title={selected?.current ? '这就是当前正文所处的版本' : '把正文回滚到该版本'}
                onClick={() => void restore()}
              >
                <RotateCcw size={14} /> {restoring ? '回滚中…' : `回滚到 v${selectedVersion ?? '-'}`}
              </button>
            </div>

            {error && <div className="auth-error">{error}</div>}

            {contentLoading ? (
              <p className="revision-empty">正在加载版本内容…</p>
            ) : selectedMarkdown === null ? (
              <p className="revision-empty">选择左侧任意版本查看内容。</p>
            ) : mode === 'source' ? (
              <pre className="revision-source">{selectedMarkdown || '（这个版本的正文是空的）'}</pre>
            ) : diff && diff.added === 0 && diff.removed === 0 ? (
              <p className="revision-empty">这个版本与当前正文完全一致。</p>
            ) : (
              <div className="revision-diff" role="table" aria-label="逐行对比">
                {chunks.map((chunk, chunkIndex) => chunk.type === 'gap' ? (
                  <div className="diff-gap" key={`gap-${chunkIndex}`}>省略 {chunk.lines} 行未改动内容</div>
                ) : (
                  chunk.rows.map((row, rowIndex) => (
                    <div className={`diff-row is-${row.kind}`} key={`row-${chunkIndex}-${rowIndex}`} role="row">
                      <span className="diff-line-number" aria-hidden="true">{row.leftLine ?? ''}</span>
                      <span className="diff-line-number" aria-hidden="true">{row.rightLine ?? ''}</span>
                      <span className="diff-sign" aria-hidden="true">
                        {row.kind === 'added' ? '+' : row.kind === 'removed' ? '−' : ' '}
                      </span>
                      <span className="diff-text">{row.text || ' '}</span>
                    </div>
                  ))
                ))}
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  )
}
