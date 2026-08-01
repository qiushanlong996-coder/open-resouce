// 搜索结果高亮片段的安全渲染。
//
// 服务端（Elasticsearch）返回的片段里混着用户内容和 <em> 标记。
// 直接把它交给 dangerouslySetInnerHTML 等于把用户内容当 HTML 执行，
// 是 XSS 入口。这里先整体转义，再只把 <em>/</em> 这一对标记还原回来——
// 除了强调标记，其余一切都当纯文本处理。
//
// 写成 .js 而不是 .ts，是为了让校验脚本能直接 import 运行
// （Node 20 不支持直接执行 TypeScript）。类型声明见同名 .d.ts。

/** @param {string} text */
function escapeHTML(text) {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

/**
 * 把高亮片段转成只保留 <em> 的安全 HTML。
 * @param {string} fragment
 * @returns {string}
 */
export function renderHighlight(fragment) {
  return escapeHTML(fragment)
    .replace(/&lt;em&gt;/g, '<em>')
    .replace(/&lt;\/em&gt;/g, '</em>')
}
