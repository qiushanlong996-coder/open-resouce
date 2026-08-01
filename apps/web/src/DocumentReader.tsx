import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Check, Copy, Link2, Search, X } from 'lucide-react'

// 阅读页的富文本渲染部件。
// 目标是对齐在线文档工具的阅读体验：代码块带语言标识和复制、图片可点击放大、
// 标题可复制锚点链接、Mermaid 图表直接渲染、Bilibili 链接内嵌播放、支持文档内搜索。

export function MermaidDiagram({ source }: { source: string }) {
  const container = useRef<HTMLDivElement | null>(null)
  const renderID = useRef(`mermaid-${Math.random().toString(36).slice(2)}`)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    setFailed(false)
    import('mermaid')
      .then(async ({ default: mermaid }) => {
        mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: 'neutral' })
        const result = await mermaid.render(renderID.current, source)
        if (!cancelled && container.current) container.current.innerHTML = result.svg
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => { cancelled = true }
  }, [source])

  // 渲染失败时保留源码，读者仍能看到图表定义。
  if (failed) {
    return <div className="mermaid-fallback">
      <span>图表无法渲染，以下是原始定义：</span>
      <pre><code>{source}</code></pre>
    </div>
  }
  return <div className="mermaid-canvas" ref={container} />
}

export function BilibiliEmbed({ url, title }: { url: string; title: string }) {
  return <div className="video-embed">
    <iframe
      src={url}
      title={title || 'Bilibili 视频'}
      loading="lazy"
      allowFullScreen
      referrerPolicy="no-referrer"
      sandbox="allow-scripts allow-same-origin allow-presentation allow-popups"
    />
  </div>
}

export function CodeBlock({
  language,
  text,
  children,
  onCopied,
}: {
  language: string
  text: string
  children: ReactNode
  onCopied: (ok: boolean) => void
}) {
  const [copied, setCopied] = useState(false)
  const copy = () => {
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(true)
      onCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    }).catch(() => onCopied(false))
  }
  return <div className="reader-code-block">
    <div className="reader-code-head">
      <span>{language || 'text'}</span>
      <button type="button" title="复制代码" onClick={copy}>
        {copied ? <Check size={13} /> : <Copy size={13} />}
        {copied ? '已复制' : '复制'}
      </button>
    </div>
    <pre><code>{children}</code></pre>
  </div>
}

export function HeadingAnchor({ id, onCopy }: { id: string; onCopy: () => void }) {
  if (!id) return null
  return <button className="heading-anchor" type="button" title="复制本节链接" onClick={onCopy}>
    <Link2 size={14} />
  </button>
}

export function ImageLightbox({ src, alt, onClose }: { src: string; alt: string; onClose: () => void }) {
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.document.addEventListener('keydown', handleKeyDown)
    return () => window.document.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  return <div className="image-lightbox" role="dialog" aria-label={alt || '图片预览'} onClick={onClose}>
    <button className="image-lightbox-close" type="button" title="关闭预览" aria-label="关闭预览">
      <X size={18} />
    </button>
    <img src={src} alt={alt} onClick={(event) => event.stopPropagation()} />
    {alt && <span className="image-lightbox-caption">{alt}</span>}
  </div>
}

export function DocumentSearchBox({
  keyword,
  onKeywordChange,
  total,
  activeIndex,
  onNext,
  onPrevious,
}: {
  keyword: string
  onKeywordChange: (value: string) => void
  total: number
  activeIndex: number
  onNext: () => void
  onPrevious: () => void
}) {
  const hint = useMemo(() => {
    if (!keyword.trim()) return ''
    return total === 0 ? '无匹配' : `${activeIndex} / ${total}`
  }, [activeIndex, keyword, total])

  return <label className="document-search">
    <Search size={14} />
    <input
      value={keyword}
      onChange={(event) => onKeywordChange(event.target.value)}
      onKeyDown={(event) => {
        if (event.key !== 'Enter') return
        event.preventDefault()
        if (event.shiftKey) onPrevious()
        else onNext()
      }}
      placeholder="在本文中搜索"
      aria-label="在本文中搜索"
    />
    {hint && <span className="document-search-hint">{hint}</span>}
    {keyword && (
      <button type="button" title="清空搜索" aria-label="清空搜索" onClick={() => onKeywordChange('')}>
        <X size={13} />
      </button>
    )}
  </label>
}
