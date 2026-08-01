import { useEffect, useMemo, useState } from 'react'

// 代码预览的语法高亮。
//
// 实现取舍：不逐行高亮。逐行调用 highlight.js 会切断跨行结构（块注释、
// 多行字符串），着色会错。这里整体高亮一次，再靠「行号独立成列 + 内容用
// white-space: pre」让两列按行高天然对齐，无需切分高亮后的 HTML。

type HighlightModule = typeof import('highlight.js/lib/common')

// 语言包体积可观，按需动态加载，读者不进代码预览就不下载。
let highlightModule: HighlightModule | null = null
let highlightLoader: Promise<HighlightModule> | null = null

function loadHighlighter() {
  highlightLoader ??= import('highlight.js/lib/common').then((module) => {
    highlightModule = module
    return module
  })
  return highlightLoader
}

function escapeHTML(text: string) {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

// useHighlightedCode 返回高亮后的 HTML。
// 语言包未就绪或语言不受支持时回退为转义后的纯文本，绝不把原始内容当 HTML 用。
export function useHighlightedCode(content: string, language: string) {
  const [ready, setReady] = useState(() => highlightModule !== null)

  useEffect(() => {
    if (highlightModule) return
    let active = true
    loadHighlighter()
      .then(() => { if (active) setReady(true) })
      .catch(() => undefined)
    return () => { active = false }
  }, [])

  return useMemo(() => {
    if (!ready || !highlightModule) return escapeHTML(content)
    try {
      if (language && highlightModule.default.getLanguage(language)) {
        return highlightModule.default.highlight(content, { language, ignoreIllegals: true }).value
      }
      // 后端给的扩展名映射不认时，交给自动识别。
      return highlightModule.default.highlightAuto(content).value
    } catch {
      return escapeHTML(content)
    }
  }, [content, language, ready])
}
