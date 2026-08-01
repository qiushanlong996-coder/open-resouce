import { Component, type ErrorInfo, type ReactNode } from 'react'

// React 错误边界。
//
// 动机来自一次真实事故：评论接口返回 `"replies":null`，前端在 setState
// 更新函数里对它调用 .map 抛出异常，异常发生在 React 处理状态的阶段，
// 绕过了 try/catch，导致整个组件树卸载、页面白屏。
//
// 边界的作用是把爆炸限制在局部：某个区块崩了，页面其余部分仍可用，
// 并给出可重试的入口，而不是整站变白屏。

type ErrorBoundaryProps = {
  // label 出现在提示文案里，便于用户和我们判断是哪一块出了问题。
  label: string
  children: ReactNode
  // onReset 在用户点击重试时调用，通常用于重新拉取数据。
  onReset?: () => void
}

type ErrorBoundaryState = {
  error: Error | null
}

export default class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // 保留堆栈到控制台，便于线上排查；这里不做上报，避免引入额外依赖。
    console.error(`[${this.props.label}] 渲染失败`, error, info.componentStack)
  }

  private reset = () => {
    this.setState({ error: null })
    this.props.onReset?.()
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children
    return (
      <div className="error-boundary" role="alert">
        <strong>{this.props.label}加载失败</strong>
        <p>这一部分出现了异常，页面其余内容仍可正常使用。</p>
        <code>{error.message || '未知错误'}</code>
        <div className="error-boundary-actions">
          <button type="button" onClick={this.reset}>重试</button>
          <button type="button" onClick={() => window.location.reload()}>刷新页面</button>
        </div>
      </div>
    )
  }
}
