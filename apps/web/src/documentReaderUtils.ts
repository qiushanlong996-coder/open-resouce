import { useEffect, useState } from 'react'

// 阅读页富文本渲染用到的纯函数与 Hook。
// 与组件分开存放，便于按职责复用，也满足 Fast Refresh 对导出内容的约束。

// bilibiliEmbedURL 把 Bilibili 视频链接转换为可嵌入播放地址。
// 一期只支持 Bilibili 外链，不支持平台内视频上传。
export function bilibiliEmbedURL(url: string): string {
  try {
    const parsed = new URL(url, window.location.origin)
    if (!/(^|\.)bilibili\.com$/.test(parsed.hostname)) return ''
    const bvid = parsed.pathname.match(/\/(BV[0-9A-Za-z]+)/)?.[1]
    if (bvid) {
      const page = parsed.searchParams.get('p') ?? '1'
      return `https://player.bilibili.com/player.html?bvid=${bvid}&page=${page}&high_quality=1&autoplay=0`
    }
    const aid = parsed.pathname.match(/\/av(\d+)/)?.[1]
    if (aid) return `https://player.bilibili.com/player.html?aid=${aid}&high_quality=1&autoplay=0`
    return ''
  } catch {
    return ''
  }
}

type SearchMatch = {
  node: Text
  start: number
  end: number
}

type HighlightRegistry = {
  CSS?: { highlights?: Map<string, unknown> }
  Highlight?: new (...ranges: Range[]) => unknown
}

// useDocumentSearch 在正文容器内做纯文本查找，用 CSS Custom Highlight API 标注命中。
// 不修改 DOM 结构，避免破坏选区评论依赖的 data-block-id 锚点。
export function useDocumentSearch(
  containerRef: { current: HTMLElement | null },
  keyword: string,
  documentKey: string,
) {
  const [matches, setMatches] = useState<SearchMatch[]>([])
  const [activeIndex, setActiveIndex] = useState(0)

  useEffect(() => {
    setActiveIndex(0)
  }, [keyword, documentKey])

  useEffect(() => {
    const container = containerRef.current
    const globals = window as unknown as HighlightRegistry
    const registry = globals.CSS?.highlights
    const HighlightConstructor = globals.Highlight

    const clear = () => {
      registry?.delete('reader-search')
      registry?.delete('reader-search-active')
    }
    if (!container || keyword.trim().length === 0) {
      clear()
      setMatches([])
      return
    }

    const needle = keyword.trim().toLowerCase()
    const found: SearchMatch[] = []
    const walker = window.document.createTreeWalker(container, NodeFilter.SHOW_TEXT)
    let node = walker.nextNode()
    while (node) {
      const text = node.textContent ?? ''
      const lower = text.toLowerCase()
      let index = lower.indexOf(needle)
      // 命中数量设上限，超长文档下避免一次性构建过多 Range。
      while (index >= 0 && found.length < 500) {
        found.push({ node: node as Text, start: index, end: index + needle.length })
        index = lower.indexOf(needle, index + needle.length)
      }
      node = walker.nextNode()
    }
    setMatches(found)

    if (!registry || !HighlightConstructor || found.length === 0) {
      clear()
      return
    }
    const ranges = found.map((match) => {
      const range = window.document.createRange()
      range.setStart(match.node, match.start)
      range.setEnd(match.node, match.end)
      return range
    })
    const current = Math.min(activeIndex, ranges.length - 1)
    registry.set('reader-search', new HighlightConstructor(...ranges) as never)
    registry.set('reader-search-active', new HighlightConstructor(ranges[current]) as never)
    return clear
  }, [activeIndex, containerRef, documentKey, keyword])

  const goto = (delta: number) => {
    if (matches.length === 0) return
    const next = (activeIndex + delta + matches.length) % matches.length
    setActiveIndex(next)
    matches[next]?.node.parentElement?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }

  return {
    total: matches.length,
    activeIndex: matches.length === 0 ? 0 : Math.min(activeIndex, matches.length - 1) + 1,
    next: () => goto(1),
    previous: () => goto(-1),
  }
}
