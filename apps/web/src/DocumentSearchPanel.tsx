import { useEffect, useRef, useState } from 'react'
import { Loader2, Search, TrendingUp, X } from 'lucide-react'
import { ApiError, getHotSearchTerms, searchDocuments, type HotSearchTerm, type SearchHit } from './api/client'
import { renderHighlight } from './searchHighlight'

// 搜索历史存在浏览器本地（按设备），不入服务端，避免记录用户查询隐私。
const HISTORY_KEY = 'openresource:search-history:v1'
const HISTORY_MAX = 8

function readHistory(): string[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY)
    const parsed = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed) ? parsed.filter((item) => typeof item === 'string').slice(0, HISTORY_MAX) : []
  } catch {
    return []
  }
}

function pushHistory(query: string): string[] {
  const trimmed = query.trim()
  if (!trimmed) return readHistory()
  const next = [trimmed, ...readHistory().filter((item) => item !== trimmed)].slice(0, HISTORY_MAX)
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(next))
  } catch {
    // localStorage 不可用（隐私模式等）时忽略，历史只是增强项。
  }
  return next
}

// 跨文档搜索面板。
//
// 与阅读页内的文内搜索不同：这里搜的是整站所有已发布项目的文档，
// 由服务端 Elasticsearch 提供，命中片段带高亮。
// 高亮片段的转义在 searchHighlight.ts 里，那里有对应的安全测试。

export default function DocumentSearchPanel({
  onOpenResult,
  onClose,
}: {
  onOpenResult: (projectSlug: string, documentSlug: string) => void
  onClose: () => void
}) {
  const [keyword, setKeyword] = useState('')
  const [hits, setHits] = useState<SearchHit[]>([])
  const [state, setState] = useState<'idle' | 'searching' | 'ready' | 'error'>('idle')
  const [error, setError] = useState('')
  const [history, setHistory] = useState<string[]>([])
  const [hotTerms, setHotTerms] = useState<HotSearchTerm[]>([])
  const inputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    inputRef.current?.focus()
    setHistory(readHistory())
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    // 加载热门搜索词（失败静默，热门词只是增强项）。
    const controller = new AbortController()
    getHotSearchTerms(controller.signal)
      .then((response) => setHotTerms(response.data))
      .catch(() => {})
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      controller.abort()
    }
  }, [onClose])

  useEffect(() => {
    const trimmed = keyword.trim()
    if (!trimmed) {
      setHits([])
      setState('idle')
      return
    }
    const controller = new AbortController()
    setState('searching')
    // 输入防抖，避免每个按键都打一次搜索。
    const timer = window.setTimeout(() => {
      searchDocuments(trimmed, controller.signal)
        .then((response) => {
          setHits(response.data)
          setState('ready')
          setHistory(pushHistory(trimmed))
        })
        .catch((reason: unknown) => {
          if (reason instanceof DOMException && reason.name === 'AbortError') return
          setHits([])
          setState('error')
          setError(reason instanceof ApiError ? reason.message : '搜索失败')
        })
    }, 300)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [keyword])

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <section
        className="search-panel"
        role="dialog"
        aria-modal="true"
        aria-label="全站文档搜索"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="search-panel-input">
          <Search size={16} />
          <input
            ref={inputRef}
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="搜索所有项目的文档内容…"
            maxLength={100}
          />
          {state === 'searching' && <Loader2 size={15} className="search-spinner" />}
          <button type="button" aria-label="关闭搜索" onClick={onClose}><X size={16} /></button>
        </div>

        <div className="search-panel-body">
          {state === 'idle' && (
            <div className="search-suggest">
              {history.length > 0 && (
                <div className="search-suggest-group">
                  <div className="search-suggest-head">
                    <span>最近搜索</span>
                    <button
                      type="button"
                      onClick={() => {
                        try { localStorage.removeItem(HISTORY_KEY) } catch { /* 忽略 */ }
                        setHistory([])
                      }}
                    >清空</button>
                  </div>
                  <div className="search-chips">
                    {history.map((term) => (
                      <button key={term} type="button" className="search-chip" onClick={() => setKeyword(term)}>{term}</button>
                    ))}
                  </div>
                </div>
              )}
              {hotTerms.length > 0 && (
                <div className="search-suggest-group">
                  <div className="search-suggest-head"><span><TrendingUp size={12} /> 热门搜索</span></div>
                  <div className="search-chips">
                    {hotTerms.map((item) => (
                      <button key={item.term} type="button" className="search-chip" onClick={() => setKeyword(item.term)}>{item.term}</button>
                    ))}
                  </div>
                </div>
              )}
              {history.length === 0 && hotTerms.length === 0 && (
                <p className="search-panel-hint">输入关键词搜索全站已发布项目的文档标题与正文。</p>
              )}
            </div>
          )}
          {state === 'error' && <p className="search-panel-hint is-error">{error}</p>}
          {state === 'ready' && hits.length === 0 && (
            <p className="search-panel-hint">没有匹配的文档。</p>
          )}
          {hits.map((hit) => (
            <button
              key={`${hit.project_slug}/${hit.document_slug}`}
              className="search-result"
              type="button"
              onClick={() => {
                onOpenResult(hit.project_slug, hit.document_slug)
                onClose()
              }}
            >
              <strong>{hit.title}</strong>
              <small>{hit.project_name}</small>
              {hit.highlight.map((fragment, index) => (
                <span
                  key={index}
                  className="search-result-snippet"
                  // 片段已在 renderHighlight 中转义，只保留 <em> 强调标记。
                  dangerouslySetInnerHTML={{ __html: renderHighlight(fragment) }}
                />
              ))}
            </button>
          ))}
        </div>

        {state === 'ready' && hits.length > 0 && (
          <div className="search-panel-foot">共 {hits.length} 条结果</div>
        )}
      </section>
    </div>
  )
}
