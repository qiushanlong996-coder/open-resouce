import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Bot, Send, Sparkles, X } from 'lucide-react'
import { getProjectAssistantURL, type AssistantChatMessage, type AuthUser } from './api/client'
import './ai-assistant.css'

type ChatMessage = AssistantChatMessage

type AiAssistantProps = {
  // 当前打开的项目。为空时不展示助手入口（助手只回答某个具体项目的问题）。
  projectSlug: string | null
  projectName: string | null
  currentUser: AuthUser | null
  onRequestLogin: () => void
}

// 历史轮数上限：只把最近若干条对话回传给后端，避免请求体无限增长。
const maxHistoryTurns = 10

// parseAssistantStream 逐段解析后端转发的 SSE（event/data 帧），
// 通过回调把文本增量、错误码与结束信号交给调用方。
async function parseAssistantStream(
  body: ReadableStream<Uint8Array>,
  handlers: { onDelta: (text: string) => void; onError: (code: string) => void; onDone: () => void },
) {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    // SSE 以空行分隔一帧，逐帧处理，剩余不完整的一帧留在缓冲区。
    let separator = buffer.indexOf('\n\n')
    while (separator !== -1) {
      const frame = buffer.slice(0, separator)
      buffer = buffer.slice(separator + 2)
      let event = 'message'
      let data = ''
      for (const line of frame.split('\n')) {
        if (line.startsWith('event:')) event = line.slice(6).trim()
        else if (line.startsWith('data:')) data += line.slice(5).trim()
      }
      if (event === 'delta') {
        try {
          const parsed = JSON.parse(data) as { text?: string }
          if (parsed.text) handlers.onDelta(parsed.text)
        } catch { /* 跳过无法解析的帧 */ }
      } else if (event === 'error') {
        let code = 'assistant_error'
        try {
          code = (JSON.parse(data) as { code?: string }).code ?? code
        } catch { /* 使用默认错误码 */ }
        handlers.onError(code)
      } else if (event === 'done') {
        handlers.onDone()
      }
      separator = buffer.indexOf('\n\n')
    }
  }
}

export default function AiAssistant({ projectSlug, projectName, currentUser, onRequestLogin }: AiAssistantProps) {
  const [open, setOpen] = useState(false)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [draft, setDraft] = useState('')
  const [streaming, setStreaming] = useState(false)
  const [unavailable, setUnavailable] = useState(false)
  const [errorText, setErrorText] = useState('')
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  // 切换项目时清空对话：助手只服务于当前打开的项目。
  useEffect(() => {
    setMessages([])
    setErrorText('')
    setUnavailable(false)
  }, [projectSlug])

  // 新消息或流式增量后滚动到底部。
  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [messages, open])

  // 组件卸载时中断进行中的请求。
  useEffect(() => () => abortRef.current?.abort(), [])

  // 项目未打开时不渲染任何入口。
  if (!projectSlug) return null

  const send = async (event: FormEvent) => {
    event.preventDefault()
    const question = draft.trim()
    if (!question || streaming) return
    if (!currentUser) {
      onRequestLogin()
      return
    }
    setErrorText('')
    const history = messages.slice(-maxHistoryTurns * 2)
    // 先落地用户消息与一个占位的助手消息，随后把增量写入占位消息。
    setMessages((previous) => [...previous, { role: 'user', content: question }, { role: 'assistant', content: '' }])
    setDraft('')
    setStreaming(true)

    const controller = new AbortController()
    abortRef.current = controller
    const appendToAssistant = (text: string) => {
      setMessages((previous) => {
        const next = [...previous]
        const last = next[next.length - 1]
        if (last && last.role === 'assistant') next[next.length - 1] = { role: 'assistant', content: last.content + text }
        return next
      })
    }

    try {
      const response = await fetch(getProjectAssistantURL(projectSlug), {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
        body: JSON.stringify({ question, history }),
        signal: controller.signal,
      })
      if (response.status === 401) {
        rollbackAssistant()
        onRequestLogin()
        return
      }
      if (response.status === 503) {
        rollbackAssistant()
        setUnavailable(true)
        return
      }
      if (!response.ok || !response.body) {
        rollbackAssistant()
        setErrorText('AI 助手暂时不可用，请稍后再试。')
        return
      }
      let failed = false
      await parseAssistantStream(response.body, {
        onDelta: appendToAssistant,
        onError: () => { failed = true },
        onDone: () => { /* 正常结束 */ },
      })
      if (failed) setErrorText('AI 助手生成回答时出错，请稍后再试。')
      // 上游没有产出任何文本时给出兜底提示，避免留下空气泡。
      setMessages((previous) => {
        const next = [...previous]
        const last = next[next.length - 1]
        if (last && last.role === 'assistant' && last.content === '' && !failed) {
          next[next.length - 1] = { role: 'assistant', content: '（没有获得回答，请换个问法再试。）' }
        }
        return next
      })
    } catch (error) {
      if ((error as Error).name === 'AbortError') return
      rollbackAssistant()
      setErrorText('网络异常，AI 助手连接失败。')
    } finally {
      setStreaming(false)
      abortRef.current = null
    }

    function rollbackAssistant() {
      // 移除占位的空助手消息，保留用户已发出的提问。
      setMessages((previous) => {
        const next = [...previous]
        const last = next[next.length - 1]
        if (last && last.role === 'assistant' && last.content === '') next.pop()
        return next
      })
    }
  }

  return (
    <div className="ai-assistant">
      {open && (
        <section className="ai-assistant-panel" aria-label="项目 AI 助手">
          <header className="ai-assistant-header">
            <div className="ai-assistant-title">
              <Sparkles size={16} />
              <div>
                <strong>项目助手</strong>
                <small>{projectName ?? '当前项目'}</small>
              </div>
            </div>
            <button className="ai-assistant-close" aria-label="关闭助手" onClick={() => setOpen(false)}>
              <X size={16} />
            </button>
          </header>

          <div className="ai-assistant-body" ref={scrollRef}>
            {unavailable ? (
              <div className="ai-assistant-notice">AI 未启用：服务端未配置 AI 助手，暂时无法回答。</div>
            ) : !currentUser ? (
              <div className="ai-assistant-notice">
                登录后即可就「{projectName ?? '该项目'}」向 AI 助手提问。
                <button className="ai-assistant-login" onClick={onRequestLogin}>去登录</button>
              </div>
            ) : messages.length === 0 ? (
              <div className="ai-assistant-empty">
                <Bot size={22} />
                <p>你好，我可以根据本项目的文档内容回答问题。试着问问它的功能或用法吧。</p>
              </div>
            ) : (
              messages.map((message, index) => (
                <div key={index} className={`ai-assistant-message ${message.role}`}>
                  <div className="ai-assistant-bubble">
                    {message.content || (streaming && index === messages.length - 1 ? '正在思考…' : '')}
                  </div>
                </div>
              ))
            )}
            {errorText && <div className="ai-assistant-error">{errorText}</div>}
          </div>

          {!unavailable && currentUser && (
            <form className="ai-assistant-composer" onSubmit={send}>
              <textarea
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault()
                    void send(event)
                  }
                }}
                placeholder="向 AI 助手提问…"
                rows={2}
                maxLength={2000}
                disabled={streaming}
              />
              <button type="submit" aria-label="发送" disabled={streaming || draft.trim().length === 0}>
                <Send size={16} />
              </button>
            </form>
          )}
        </section>
      )}

      <button
        className="ai-assistant-fab"
        aria-label={open ? '关闭 AI 助手' : '打开 AI 助手'}
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        {open ? <X size={20} /> : <Sparkles size={20} />}
      </button>
    </div>
  )
}
