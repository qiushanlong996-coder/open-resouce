#!/usr/bin/env node
// 搜索高亮转义的安全校验。
//
// renderHighlight 是一道 XSS 防线：ES 返回的片段里混着用户写的正文，
// 会被交给 dangerouslySetInnerHTML。如果转义写错，用户在文档里写一段
// <script> 就能在别人搜索时执行。这条防线必须有测试守住。
//
// 前端没有测试框架，这里用可直接执行的脚本做断言。
// 用法：node apps/web/scripts/check-search-highlight.mjs

import { renderHighlight } from '../src/searchHighlight.js'

let failures = 0
function check(label, actual, expected) {
  const ok = actual === expected
  console.log(`${ok ? 'PASS' : 'FAIL'} ${label}`)
  if (!ok) {
    console.log(`  实际: ${actual}`)
    console.log(`  期望: ${expected}`)
    failures += 1
  }
}
function mustNotContain(label, actual, forbidden) {
  const ok = !actual.includes(forbidden)
  console.log(`${ok ? 'PASS' : 'FAIL'} ${label}`)
  if (!ok) {
    console.log(`  输出里出现了不该有的 ${forbidden}: ${actual}`)
    failures += 1
  }
}

console.log('=== 保留 ES 的 <em> 强调标记 ===')
check('纯文本原样', renderHighlight('使用指南'), '使用指南')
check('em 标记保留', renderHighlight('<em>使用</em>指南'), '<em>使用</em>指南')
check('多处 em', renderHighlight('<em>a</em>b<em>c</em>'), '<em>a</em>b<em>c</em>')

console.log('=== 拦截脚本注入 ===')
mustNotContain('script 标签被转义', renderHighlight('<script>alert(1)</script>'), '<script>')
check('script 转义结果',
  renderHighlight('<script>alert(1)</script>'),
  '&lt;script&gt;alert(1)&lt;/script&gt;')
mustNotContain('img onerror 被转义',
  renderHighlight('<img src=x onerror=alert(1)>'), '<img')
mustNotContain('iframe 被转义',
  renderHighlight('<iframe src="javascript:alert(1)"></iframe>'), '<iframe')
mustNotContain('svg onload 被转义',
  renderHighlight('<svg onload=alert(1)>'), '<svg')

console.log('=== 属性注入与引号 ===')
check('双引号被转义', renderHighlight('say "hi"'), 'say &quot;hi&quot;')
// 带属性的 em 不会被还原：只有完全等于 <em> 和 </em> 的标记才放行。
// 这里断言的是“没有产生可执行的开标签”，而不是“输出里不能出现
// onmouseover 这个词”——后者过严：该词作为纯文本出现是安全的。
check('带属性的 em 被整体转义成文本',
  renderHighlight('<em onmouseover=alert(1)>x'),
  '&lt;em onmouseover=alert(1)&gt;x')
mustNotContain('不产生带属性的开标签',
  renderHighlight('<em onmouseover=alert(1)>x'), '<em ')

console.log('=== & 的处理顺序（先转义 & 才不会二次转义）===')
check('单个 &', renderHighlight('a & b'), 'a &amp; b')
check('已转义实体不被破坏', renderHighlight('&amp;'), '&amp;amp;')

console.log('=== 混合场景：用户内容里写了 em 字面量 ===')
// 用户正文里若本来就写了 <em>，与 ES 生成的标记无法区分，这是已知取舍：
// 两者都会被当成强调标记。代价可接受——<em> 本身无副作用。
check('用户写的 em 也被当强调', renderHighlight('<em>x</em>'), '<em>x</em>')
// 但危险标签绝不放行。
mustNotContain('em 之外的标签一律转义',
  renderHighlight('<em>ok</em><script>bad</script>'), '<script>')

console.log(failures === 0 ? 'RESULT=ALL_PASS' : `RESULT=HAS_FAILURE(${failures})`)
process.exit(failures === 0 ? 0 : 1)
