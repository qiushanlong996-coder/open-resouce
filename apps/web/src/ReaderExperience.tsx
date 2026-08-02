import { useEffect, useId, useRef, useState } from 'react'
import { ArrowUp, ChevronLeft, ChevronRight, RotateCcw, SlidersHorizontal, X } from 'lucide-react'
import {
  prefersReducedMotion,
  type FlatDocument,
  type ReaderFontFamily,
  type ReaderFontSize,
  type ReaderLineHeight,
  type ReaderPreferencesController,
  type ReaderWidth,
} from './readerExperienceUtils'
import './reader-experience.css'

// 阅读体验增强的展示组件。CSS 单独放在 reader-experience.css，由本文件导入，
// 不改动 App.css 的主题变量；样式统一以 var(--blue) 等主题变量取色，适配浅色/深色与全部皮肤。

// —— 阅读进度条 ——
// 顶部细进度条，宽度反映正文滚动进度（0~1）。纯展示，进度由父组件用 useScrollMetrics 计算。
export function ReadingProgressBar({ progress }: { progress: number }) {
  const pct = Math.round(Math.min(1, Math.max(0, progress)) * 1000) / 10
  return (
    <div className="reader-progress" role="progressbar" aria-label="阅读进度" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)}>
      <span className="reader-progress-fill" style={{ width: `${pct}%` }} />
    </div>
  )
}

// —— 回到顶部 ——
export function BackToTopButton({ visible }: { visible: boolean }) {
  const toTop = () => {
    window.scrollTo({ top: 0, behavior: prefersReducedMotion() ? 'auto' : 'smooth' })
  }
  return (
    <button
      type="button"
      className={`reader-back-to-top ${visible ? 'is-visible' : ''}`}
      title="回到顶部"
      aria-label="回到顶部"
      aria-hidden={!visible}
      tabIndex={visible ? 0 : -1}
      onClick={toTop}
    >
      <ArrowUp size={18} />
    </button>
  )
}

// —— 上一篇 / 下一篇 ——
// 依据展开后的文档顺序渲染前后导航，单篇文档时不渲染。
export function DocumentPager({
  items,
  currentSlug,
  onOpen,
}: {
  items: FlatDocument[]
  currentSlug: string
  onOpen: (slug: string) => void
}) {
  if (items.length <= 1) return null
  const index = items.findIndex((item) => item.slug === currentSlug)
  if (index < 0) return null
  const prev = index > 0 ? items[index - 1] : null
  const next = index < items.length - 1 ? items[index + 1] : null
  if (!prev && !next) return null

  return (
    <nav className="reader-pager" aria-label="上一篇 / 下一篇">
      {prev ? (
        <button type="button" className="reader-pager-link prev" onClick={() => onOpen(prev.slug)}>
          <ChevronLeft size={16} />
          <span>
            <small>上一篇</small>
            <strong>{prev.title}</strong>
          </span>
        </button>
      ) : <span className="reader-pager-spacer" aria-hidden="true" />}
      {next ? (
        <button type="button" className="reader-pager-link next" onClick={() => onOpen(next.slug)}>
          <span>
            <small>下一篇</small>
            <strong>{next.title}</strong>
          </span>
          <ChevronRight size={16} />
        </button>
      ) : <span className="reader-pager-spacer" aria-hidden="true" />}
    </nav>
  )
}

// —— 阅读设置 ——
type SegOption<T extends string> = { value: T; label: string }

function SegmentedControl<T extends string>({
  legend,
  value,
  options,
  onSelect,
}: {
  legend: string
  value: T
  options: SegOption<T>[]
  onSelect: (value: T) => void
}) {
  return (
    <div className="reader-setting-row" role="group" aria-label={legend}>
      <span className="reader-setting-label">{legend}</span>
      <div className="reader-segment">
        {options.map((option) => (
          <button
            key={option.value}
            type="button"
            className={value === option.value ? 'active' : ''}
            aria-pressed={value === option.value}
            onClick={() => onSelect(option.value)}
          >
            {option.label}
          </button>
        ))}
      </div>
    </div>
  )
}

const FONT_SIZE_OPTIONS: SegOption<ReaderFontSize>[] = [
  { value: 'small', label: '小' },
  { value: 'medium', label: '中' },
  { value: 'large', label: '大' },
]
const LINE_HEIGHT_OPTIONS: SegOption<ReaderLineHeight>[] = [
  { value: 'tight', label: '紧凑' },
  { value: 'cozy', label: '舒适' },
  { value: 'loose', label: '宽松' },
]
const WIDTH_OPTIONS: SegOption<ReaderWidth>[] = [
  { value: 'narrow', label: '窄' },
  { value: 'medium', label: '中' },
  { value: 'wide', label: '宽' },
]
const FONT_FAMILY_OPTIONS: SegOption<ReaderFontFamily>[] = [
  { value: 'sans', label: '无衬线' },
  { value: 'serif', label: '衬线' },
]

export function ReaderSettingsControl({ controller }: { controller: ReaderPreferencesController }) {
  const { preferences, update, reset, isDefault } = controller
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLDivElement | null>(null)
  const panelId = useId()

  // 点击外部或按 Esc 关闭弹层；关闭后焦点交还给触发按钮由浏览器默认行为处理。
  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(event.target as Node)) setOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    window.document.addEventListener('mousedown', onPointerDown)
    window.document.addEventListener('keydown', onKeyDown)
    return () => {
      window.document.removeEventListener('mousedown', onPointerDown)
      window.document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  return (
    <div className="reader-settings" ref={wrapRef}>
      <button
        type="button"
        className={`tool-button ${open ? 'is-active' : ''}`}
        title="阅读设置"
        aria-label="阅读设置"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        onClick={() => setOpen((prev) => !prev)}
      >
        <SlidersHorizontal size={14} /> 阅读设置
      </button>
      {open && (
        <div className="reader-settings-popover" id={panelId} role="dialog" aria-label="阅读设置">
          <div className="reader-settings-head">
            <span>阅读设置</span>
            <button type="button" className="reader-settings-close" title="关闭" aria-label="关闭阅读设置" onClick={() => setOpen(false)}>
              <X size={14} />
            </button>
          </div>
          <SegmentedControl legend="字号" value={preferences.fontSize} options={FONT_SIZE_OPTIONS} onSelect={(value) => update('fontSize', value)} />
          <SegmentedControl legend="行距" value={preferences.lineHeight} options={LINE_HEIGHT_OPTIONS} onSelect={(value) => update('lineHeight', value)} />
          <SegmentedControl legend="宽度" value={preferences.width} options={WIDTH_OPTIONS} onSelect={(value) => update('width', value)} />
          <SegmentedControl legend="字体" value={preferences.fontFamily} options={FONT_FAMILY_OPTIONS} onSelect={(value) => update('fontFamily', value)} />
          <button type="button" className="reader-settings-reset" disabled={isDefault} onClick={reset}>
            <RotateCcw size={13} /> 恢复默认
          </button>
        </div>
      )}
    </div>
  )
}
