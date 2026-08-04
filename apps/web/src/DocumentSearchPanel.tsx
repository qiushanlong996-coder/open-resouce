import { useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import { CornerDownLeft, FileText, Loader2, Search, TrendingUp, X } from 'lucide-react'
import { ApiError, getHotSearchTerms, searchDocuments, type HotSearchTerm, type SearchHit } from './api/client'
import { renderHighlight } from './searchHighlight'
import './search-panel.css'

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

// projectGroup 是按项目聚合后的一组命中。
type projectGroup = {
  projectSlug: string
  projectName: string
  summary: string
  category: string
  hits: SearchHit[]
}

// groupByProject 把平铺的命中按项目聚合，组内保持服务端的相关度顺序，
// 组间按各自最高分排序。一次接口调用就能同时给出「项目」和「文档」两层结构，
// 不需要第二个数据源。
function groupByProject(hits: SearchHit[]): projectGroup[] {
  const groups: projectGroup[] = []
  const index = new Map<string, projectGroup>()
  for (const hit of hits) {
    let group = index.get(hit.project_slug)
    if (!group) {
      group = {
        projectSlug: hit.project_slug,
        projectName: hit.project_name,
        summary: hit.project_summary ?? '',
        category: hit.category ?? '',
        hits: [],
      }
      index.set(hit.project_slug, group)
      groups.push(group)
    }
    group.hits.push(hit)
  }
  return groups
}

// flatRow 是键盘导航用的扁平序列：组标题和其下的文档都可被选中。
type flatRow =
  | { kind: 'project'; group: projectGroup }
  | { kind: 'document'; group: projectGroup; hit: SearchHit }

function flattenRows(groups: projectGroup[]): flatRow[] {
  return groups.flatMap((group) => [
    { kind: 'project' as const, group },
    ...group.hits.map((hit) => ({ kind: 'document' as const, group, hit })),
  ])
}

// 全站搜索面板。
//
// 搜的是所有已发布项目的文档标题、正文，以及项目名、简介、分类和标签，
// 由服务端 Elasticsearch 提供，命中片段带高亮。
// 高亮片段的转义在 searchHighlight.ts 里，那里有对应的安全测试。
//
// 与阅读页内的文内搜索（DocumentSearchBox）不同：那个只搜当前这一篇。

export default function DocumentSearchPanel({
  onOpenResult,
  onOpenProject,
  onClose,
}: {
  onOpenResult: (projectSlug: string, documentSlug: string) => void
  onOpenProject: (projectSlug: string) => void
  onClose: () => void
}) {
  const [keyword, setKeyword] = useState('')
  const [hits, setHits] = useState<SearchHit[]>([])
  const [state, setState] = useState<'idle' | 'searching' | 'ready' | 'error'>('idle')
  const [error, setError] = useState('')
  const [history, setHistory] = useState<string[]>([])
  const [hotTerms, setHotTerms] = useState<HotSearchTerm[]>([])
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement | null>(null)
  const listRef = useRef<HTMLDivElement | null>(null)

  const groups = useMemo(() => groupByProject(hits), [hits])
  const rows = useMemo(() => flattenRows(groups), [groups])

  useEffect(() => {
    inputRef.current?.focus()
    setHistory(readHistory())
    // 加载热门搜索词（失败静默，热门词只是增强项）。
    const controller = new AbortController()
    getHotSearchTerms(controller.signal)
      .then((response) => setHotTerms(response.data))
      .catch(() => {})
    return () => controller.abort()
  }, [])

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
          setActiveIndex(0)
          setHistory(pushHistory(trimmed))
        })
        .catch((reason: unknown) => {
          if (reason instanceof DOMException && reason.name === 'AbortError') return
          setHits([])
          setState('error')
          setError(reason instanceof ApiError ? reason.message : '搜索失败')
        })
    }, 250)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [keyword])

  const openRow = (row: flatRow) => {
    if (row.kind === 'project') onOpenProject(row.group.projectSlug)
    else onOpenResult(row.group.projectSlug, row.hit.document_slug)
    onClose()
  }

  // ↑↓ 移动、Enter 打开、Esc 关闭。事件绑在面板上而不是 window，
  // 这样输入法候选窗的按键不会被误当成导航。
  const onKeyDown = (event: ReactKeyboardEvent) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (rows.length === 0) return
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveIndex((current) => (current + 1) % rows.length)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveIndex((current) => (current - 1 + rows.length) % rows.length)
    } else if (event.key === 'Enter') {
      // 输入法组合输入中的回车是「上屏」，不是「打开结果」。
      if (event.nativeEvent.isComposing) return
      event.preventDefault()
      const row = rows[activeIndex]
      if (row) openRow(row)
    }
  }

  // 选中项滚进可视区域，键盘导航到列表外时不至于看不见。
  useEffect(() => {
    const active = listRef.current?.querySelector<HTMLElement>('[data-active="true"]')
    active?.scrollIntoView({ block: 'nearest' })
  }, [activeIndex])

  const showSuggestions = state === 'idle' && (history.length > 0 || hotTerms.length > 0)

  return (
    <div className="search-overlay" role="presentation" onMouseDown={onClose}>
      <section
        className="search-panel"
        role="dialog"
        aria-modal="true"
        aria-label="搜索项目与文档"
        onMouseDown={(event) => event.stopPropagation()}
        onKeyDown={onKeyDown}
      >
        <div className="search-panel-input">
          <Search size={17} aria-hidden="true" />
          <input
            ref={inputRef}
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="搜索项目、文档、标签…"
            maxLength={100}
            aria-label="搜索关键词"
          />
          {state === 'searching' && <Loader2 size={15} className="search-spinner" aria-hidden="true" />}
          <button className="search-panel-close" type="button" aria-label="关闭搜索" onClick={onClose}>
            <X size={16} />
          </button>
        </div>

        <div className="search-panel-body" ref={listRef}>
          {showSuggestions && (
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
            </div>
          )}
          {state === 'idle' && !showSuggestions && (
            <p className="search-panel-hint">
              输入关键词，搜索全站已发布项目的名称、简介、标签，以及文档标题与正文。
            </p>
          )}
          {state === 'error' && <p className="search-panel-hint is-error">{error}</p>}
          {state === 'ready' && hits.length === 0 && (
            <p className="search-panel-hint">
              没有找到与「{keyword.trim()}」相关的内容。换个更短的词试试。
            </p>
          )}

          {groups.map((group) => {
            const projectRow = rows.findIndex(
              (row) => row.kind === 'project' && row.group.projectSlug === group.projectSlug,
            )
            return (
              <div className="search-group" key={group.projectSlug}>
                <button
                  type="button"
                  className="search-group-head"
                  data-active={activeIndex === projectRow}
                  onMouseEnter={() => setActiveIndex(projectRow)}
                  onClick={() => { onOpenProject(group.projectSlug); onClose() }}
                >
                  <span className="search-group-mark" aria-hidden="true">{group.projectName.slice(0, 1)}</span>
                  <span className="search-group-text">
                    <strong>{group.projectName}</strong>
                    {group.summary && <small>{group.summary}</small>}
                  </span>
                  {group.category && <span className="search-group-category">{group.category}</span>}
                </button>
                {group.hits.map((hit) => {
                  const rowIndex = rows.findIndex(
                    (row) => row.kind === 'document' && row.hit === hit,
                  )
                  return (
                    <button
                      key={`${hit.project_slug}/${hit.document_slug}`}
                      className="search-result"
                      type="button"
                      data-active={activeIndex === rowIndex}
                      onMouseEnter={() => setActiveIndex(rowIndex)}
                      onClick={() => {
                        onOpenResult(hit.project_slug, hit.document_slug)
                        onClose()
                      }}
                    >
                      <span className="search-result-title">
                        <FileText size={13} aria-hidden="true" />
                        <strong>{hit.title}</strong>
                      </span>
                      {hit.highlight.slice(0, 2).map((fragment, index) => (
                        <span
                          key={index}
                          className="search-result-snippet"
                          // 片段已在 renderHighlight 中转义，只保留 <em> 强调标记。
                          dangerouslySetInnerHTML={{ __html: renderHighlight(fragment) }}
                        />
                      ))}
                    </button>
                  )
                })}
              </div>
            )
          })}
        </div>

        <div className="search-panel-foot">
          <span>
            {state === 'ready' && hits.length > 0
              ? `${hits.length} 条结果 · ${groups.length} 个项目`
              : '全站检索'}
          </span>
          <span className="search-panel-keys">
            <kbd>↑</kbd><kbd>↓</kbd> 选择
            <kbd><CornerDownLeft size={10} /></kbd> 打开
            <kbd>esc</kbd> 关闭
          </span>
        </div>
      </section>
    </div>
  )
}
