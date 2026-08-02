import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import ErrorBoundary from './ErrorBoundary.tsx'
// 主题背景接线层放在 App（含 App.css）之后导入，确保 .app-shell 背景规则生效。
import './themes.css'
// 品牌资源样式最后导入，覆盖 App.css 中的品牌标记与头像默认值。
import './brand.css'
import './visual-refresh.css'

// 最外层兜底边界。局部边界已覆盖各功能区块，这一层只处理它们之外的
// 意外崩溃，保证任何情况下都给出可操作的提示而不是一片空白。
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary label="页面">
      <App />
    </ErrorBoundary>
  </StrictMode>,
)
