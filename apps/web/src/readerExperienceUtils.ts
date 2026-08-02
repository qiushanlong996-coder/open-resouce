import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import type { DocumentNode, DocumentOutlineItem } from './api/client'

// 阅读体验增强的纯函数与 Hook。
// 与展示组件分离存放，既方便按职责复用，也满足 oxlint 的 react/only-export-components：
// 本文件不导出任何组件，因此不会触发 Fast Refresh 相关告警。

// —— 阅读设置 ——
// 读者可调节的正文排版偏好，持久化到 localStorage，运行时以 CSS 变量作用于正文容器。
export type ReaderFontSize = 'small' | 'medium' | 'large'
export type ReaderLineHeight = 'tight' | 'cozy' | 'loose'
export type ReaderWidth = 'narrow' | 'medium' | 'wide'
export type ReaderFontFamily = 'sans' | 'serif'

export type ReaderPreferences = {
  fontSize: ReaderFontSize
  lineHeight: ReaderLineHeight
  width: ReaderWidth
  fontFamily: ReaderFontFamily
}

// 持久化键，沿用项目既有的 xinyuan- 前缀（见主题模式/皮肤）。
export const READER_PREFS_STORAGE_KEY = 'xinyuan-reader-prefs'

const DEFAULT_PREFERENCES: ReaderPreferences = {
  fontSize: 'medium',
  lineHeight: 'cozy',
  width: 'medium',
  fontFamily: 'sans',
}

// 各档位对应的具体取值。字号刻意保守（14/15/17），避免破坏代码块与表格的既有排版。
const FONT_SIZE_PX: Record<ReaderFontSize, string> = { small: '14px', medium: '15px', large: '17px' }
const LINE_HEIGHT: Record<ReaderLineHeight, string> = { tight: '1.6', cozy: '1.8', loose: '2.05' }
const MAX_WIDTH_PX: Record<ReaderWidth, string> = { narrow: '560px', medium: '650px', wide: '820px' }
const FONT_FAMILY: Record<ReaderFontFamily, string> = {
  sans: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  serif: '"Georgia", "Songti SC", "Noto Serif SC", "SimSun", serif',
}

function isOption<T extends string>(value: unknown, table: Record<T, unknown>): value is T {
  return typeof value === 'string' && Object.prototype.hasOwnProperty.call(table, value)
}

// sanitizePreferences 校验读取到的数据，任一字段非法都回退默认值，避免旧数据或手改导致的异常。
function sanitizePreferences(raw: unknown): ReaderPreferences {
  const source = (raw && typeof raw === 'object' ? raw : {}) as Partial<Record<keyof ReaderPreferences, unknown>>
  return {
    fontSize: isOption(source.fontSize, FONT_SIZE_PX) ? source.fontSize : DEFAULT_PREFERENCES.fontSize,
    lineHeight: isOption(source.lineHeight, LINE_HEIGHT) ? source.lineHeight : DEFAULT_PREFERENCES.lineHeight,
    width: isOption(source.width, MAX_WIDTH_PX) ? source.width : DEFAULT_PREFERENCES.width,
    fontFamily: isOption(source.fontFamily, FONT_FAMILY) ? source.fontFamily : DEFAULT_PREFERENCES.fontFamily,
  }
}

export type ReaderPreferencesController = {
  preferences: ReaderPreferences
  update: <K extends keyof ReaderPreferences>(key: K, value: ReaderPreferences[K]) => void
  reset: () => void
  isDefault: boolean
  // 作用于正文容器的内联样式，仅包含 CSS 自定义属性，由 reader-experience.css 消费。
  style: CSSProperties
}

export function useReaderPreferences(): ReaderPreferencesController {
  const [preferences, setPreferences] = useState<ReaderPreferences>(() => {
    try {
      const stored = window.localStorage.getItem(READER_PREFS_STORAGE_KEY)
      return stored ? sanitizePreferences(JSON.parse(stored)) : DEFAULT_PREFERENCES
    } catch {
      return DEFAULT_PREFERENCES
    }
  })

  useEffect(() => {
    try {
      window.localStorage.setItem(READER_PREFS_STORAGE_KEY, JSON.stringify(preferences))
    } catch {
      // 无痕模式或存储被禁用时静默失败，设置仍在本次会话内生效。
    }
  }, [preferences])

  const update = useCallback<ReaderPreferencesController['update']>((key, value) => {
    setPreferences((prev) => ({ ...prev, [key]: value }))
  }, [])

  const reset = useCallback(() => setPreferences(DEFAULT_PREFERENCES), [])

  const style = useMemo<CSSProperties>(() => ({
    '--reader-font-size': FONT_SIZE_PX[preferences.fontSize],
    '--reader-line-height': LINE_HEIGHT[preferences.lineHeight],
    '--reader-max-width': MAX_WIDTH_PX[preferences.width],
    '--reader-font-family': FONT_FAMILY[preferences.fontFamily],
  } as CSSProperties), [preferences])

  const isDefault = useMemo(
    () => (Object.keys(DEFAULT_PREFERENCES) as (keyof ReaderPreferences)[]).every((key) => preferences[key] === DEFAULT_PREFERENCES[key]),
    [preferences],
  )

  return { preferences, update, reset, isDefault, style }
}

// —— 目录滚动高亮（scroll-spy）——
// 用 IntersectionObserver 观察正文里的标题元素，返回当前最靠近视口顶部的可见标题 id。
// 只读观察、不改 DOM，因此不影响选区评论锚点（data-block-id）与文档内搜索的高亮。
export function useScrollSpy(outline: DocumentOutlineItem[] | undefined, documentKey: string): string {
  const [activeId, setActiveId] = useState('')
  // 以 id 序列作为稳定依赖，避免每次渲染都重建 observer。
  const idsKey = useMemo(() => (outline ?? []).map((item) => item.id).join('|'), [outline])

  useEffect(() => {
    setActiveId('')
    const ids = idsKey ? idsKey.split('|') : []
    if (ids.length === 0 || typeof IntersectionObserver === 'undefined') return

    const elements = ids
      .map((id) => window.document.getElementById(id))
      .filter((el): el is HTMLElement => el !== null)
    if (elements.length === 0) return

    // 记录每个标题当前相对触发线（视口顶部下移 84px）的位置，选取已滚过且最靠下的一个。
    const tops = new Map<string, number>()
    const pickActive = () => {
      let best = ''
      let bestTop = Number.NEGATIVE_INFINITY
      tops.forEach((top, id) => {
        if (top <= 0 && top > bestTop) {
          bestTop = top
          best = id
        }
      })
      if (!best) {
        // 还没有标题滚过触发线（停在文首）时，高亮位置最靠上的可见标题。
        let firstTop = Number.POSITIVE_INFINITY
        tops.forEach((top, id) => {
          if (top < firstTop) {
            firstTop = top
            best = id
          }
        })
      }
      if (best) setActiveId(best)
    }

    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        tops.set(entry.target.id, entry.boundingClientRect.top - 84)
      }
      pickActive()
    }, { rootMargin: '-84px 0px -55% 0px', threshold: [0, 1] })

    elements.forEach((el) => observer.observe(el))
    return () => observer.disconnect()
  }, [idsKey, documentKey])

  return activeId
}

// —— 上一篇 / 下一篇 ——
// 把目录树按深度优先展开成有序的可读文档列表，复用与 firstDocumentSlug 一致的遍历顺序。
export type FlatDocument = { slug: string; title: string }

export function flattenDocumentTree(nodes: DocumentNode[]): FlatDocument[] {
  const result: FlatDocument[] = []
  const walk = (list: DocumentNode[]) => {
    for (const node of list) {
      if (node.slug) result.push({ slug: node.slug, title: node.title })
      if (node.children?.length) walk(node.children)
    }
  }
  walk(nodes)
  return result
}

// —— 阅读进度 / 回到顶部 ——
// prefersReducedMotion 用于在“减少动态效果”时退化为瞬时滚动。
export function prefersReducedMotion(): boolean {
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

// useScrollMetrics 计算目标容器的阅读进度（0~1）与是否已向下滚动一定距离。
// 用 requestAnimationFrame 节流 scroll/resize，卸载时清理监听。
export function useScrollMetrics(targetRef: { current: HTMLElement | null }): { progress: number; scrolled: boolean } {
  const [progress, setProgress] = useState(0)
  const [scrolled, setScrolled] = useState(false)
  const frame = useRef(0)

  useEffect(() => {
    const measure = () => {
      frame.current = 0
      const el = targetRef.current
      if (!el) {
        setProgress(0)
        setScrolled(window.scrollY > 480)
        return
      }
      const start = el.offsetTop
      const span = el.offsetHeight - window.innerHeight
      const passed = window.scrollY - start
      const ratio = span > 0 ? passed / span : (window.scrollY > start ? 1 : 0)
      setProgress(Math.min(1, Math.max(0, ratio)))
      setScrolled(window.scrollY > 480)
    }
    const schedule = () => {
      if (frame.current) return
      frame.current = window.requestAnimationFrame(measure)
    }
    measure()
    window.addEventListener('scroll', schedule, { passive: true })
    window.addEventListener('resize', schedule)
    return () => {
      if (frame.current) window.cancelAnimationFrame(frame.current)
      window.removeEventListener('scroll', schedule)
      window.removeEventListener('resize', schedule)
    }
  }, [targetRef])

  return { progress, scrolled }
}
