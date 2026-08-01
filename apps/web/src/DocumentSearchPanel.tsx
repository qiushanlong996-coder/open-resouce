import { useEffect, useRef, useState } from 'react'
import { Loader2, Search, X } from 'lucide-react'
import { ApiError, searchDocuments, type SearchHit } from './api/client'
import { renderHighlight } from './searchHighlight'

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
  const inputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    inputRef.current?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
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
            <p className="search-panel-hint">输入关键词搜索全站已发布项目的文档标题与正文。</p>
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
