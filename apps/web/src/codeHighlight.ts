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

// splitHighlightedLines 把整体高亮后的 HTML 拆成「每行一个 HTML 片段」。
//
// 为什么不逐行高亮：见文件顶部说明。这里在拆行时维护一个「当前打开的标签栈」，
// 遇到换行就先按栈逆序补齐 </span> 收尾当前行，换行后再把这些标签原样重开，
// 从而在保持跨行着色正确的同时，让每一行都是自洽、可安全注入的 HTML。
// highlight.js 的公共产物只包含 <span class="hljs-…"> 与转义文本，正则分词即可覆盖；
// 输入本身已被 useHighlightedCode 转义，拆分不会引入新的可执行 HTML。
export function splitHighlightedLines(html: string): string[] {
  const tokenizer = /(<span[^>]*>)|(<\/span>)|([^<]+)/g
  const lines: string[] = []
  const openStack: string[] = []
  let current = ''
  let match: RegExpExecArray | null
  while ((match = tokenizer.exec(html)) !== null) {
    const [, open, close, text] = match
    if (open) {
      openStack.push(open)
      current += open
    } else if (close) {
      openStack.pop()
      current += close
    } else if (text != null) {
      const parts = text.split('\n')
      for (let index = 0; index < parts.length; index += 1) {
        if (index > 0) {
          for (let depth = openStack.length - 1; depth >= 0; depth -= 1) current += '</span>'
          lines.push(current)
          current = openStack.join('')
        }
        current += parts[index]
      }
    }
  }
  lines.push(current)
  return lines
}

// useHighlightedLines 返回逐行高亮片段，行数与源码行数一致时才采用，
// 否则回退到「按行转义纯文本」，保证行号与内容严格对齐、绝不错位。
export function useHighlightedLines(content: string, language: string) {
  const html = useHighlightedCode(content, language)
  return useMemo(() => {
    const sourceLines = content.split('\n')
    const highlighted = splitHighlightedLines(html)
    if (highlighted.length === sourceLines.length) return highlighted
    return sourceLines.map((line) => escapeHTML(line))
  }, [html, content])
}
