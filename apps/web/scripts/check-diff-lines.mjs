#!/usr/bin/env node
// 历史版本逐行对比的正确性校验。
//
// 回滚是破坏性操作：作者按对比结果决定要不要用旧版本覆盖当前正文。
// 如果 diff 把「没改的行」标成删除，或漏报真正被删掉的段落，
// 作者就会基于错误信息做出错误的回滚决定，所以这条逻辑必须有断言守住。
//
// 前端没有测试框架，这里用可直接执行的脚本做断言。
// 用法：node apps/web/scripts/check-diff-lines.mjs

import { collapseDiffRows, diffLines } from '../src/diffLines.js'

let failures = 0
function check(label, actual, expected) {
  const ok = JSON.stringify(actual) === JSON.stringify(expected)
  console.log(`${ok ? 'PASS' : 'FAIL'} ${label}`)
  if (!ok) {
    console.log(`  实际: ${JSON.stringify(actual)}`)
    console.log(`  期望: ${JSON.stringify(expected)}`)
    failures += 1
  }
}

const kinds = (result) => result.rows.map((row) => row.kind)
const texts = (result, kind) => result.rows.filter((row) => row.kind === kind).map((row) => row.text)

console.log('=== 完全相同的正文没有任何增删 ===')
{
  const result = diffLines('# 标题\n\n正文', '# 标题\n\n正文')
  check('增删计数为 0', [result.added, result.removed], [0, 0])
  check('全部标为 equal', kinds(result), ['equal', 'equal', 'equal'])
}

console.log('=== 只在中间插入一行 ===')
{
  const result = diffLines('a\nb', 'a\nx\nb')
  check('只算一处新增', [result.added, result.removed], [1, 0])
  check('新增内容是 x', texts(result, 'added'), ['x'])
  check('顺序为 a x b', kinds(result), ['equal', 'added', 'equal'])
}

console.log('=== 删除中间一行 ===')
{
  const result = diffLines('a\nx\nb', 'a\nb')
  check('只算一处删除', [result.added, result.removed], [0, 1])
  check('删除内容是 x', texts(result, 'removed'), ['x'])
}

console.log('=== 修改一行等于一删一增 ===')
{
  const result = diffLines('a\n旧内容\nb', 'a\n新内容\nb')
  check('一删一增', [result.added, result.removed], [1, 1])
  check('删除行在新增行之前', kinds(result), ['equal', 'removed', 'added', 'equal'])
}

console.log('=== 行号只在自己一侧出现 ===')
{
  const result = diffLines('a\nx', 'a\ny')
  const removed = result.rows.find((row) => row.kind === 'removed')
  const added = result.rows.find((row) => row.kind === 'added')
  check('删除行只有左侧行号', [removed.leftLine, removed.rightLine], [2, null])
  check('新增行只有右侧行号', [added.leftLine, added.rightLine], [null, 2])
  const equal = result.rows[0]
  check('未改动行两侧都有行号', [equal.leftLine, equal.rightLine], [1, 1])
}

console.log('=== 空正文的边界情况 ===')
{
  const fromEmpty = diffLines('', 'a\nb')
  check('从空到两行是两处新增', [fromEmpty.added, fromEmpty.removed], [2, 0])
  const toEmpty = diffLines('a\nb', '')
  check('清空两行是两处删除', [toEmpty.added, toEmpty.removed], [0, 2])
  const bothEmpty = diffLines('', '')
  check('两边都空没有任何行', bothEmpty.rows.length, 0)
}

console.log('=== 超长文档退化为整块替换而不是卡死 ===')
{
  const long = Array.from({ length: 2500 }, (_, index) => `行 ${index}`).join('\n')
  const result = diffLines(long, long)
  check('标记为已截断', result.truncated, true)
  check('整块替换：全删全增', [result.added, result.removed], [2500, 2500])
}

console.log('=== 折叠大段未改动内容 ===')
{
  const before = Array.from({ length: 30 }, (_, index) => `行 ${index}`).join('\n')
  const after = before.replace('行 15', '行 15 改过了')
  const result = diffLines(before, after)
  const chunks = collapseDiffRows(result.rows, 3)
  const gaps = chunks.filter((chunk) => chunk.type === 'gap')
  check('前后各有一段被折叠', gaps.length, 2)
  const shown = chunks
    .filter((chunk) => chunk.type === 'rows')
    .flatMap((chunk) => chunk.rows)
  check('改动行仍然可见', shown.some((row) => row.text === '行 15 改过了'), true)
  check('折叠后总行数等于展示加省略',
    shown.length + gaps.reduce((total, gap) => total + gap.lines, 0),
    result.rows.length)
  // 有改动时不能把改动点连同上下文一起折走。
  const contextTexts = shown.map((row) => row.text)
  check('保留改动前 3 行上下文', contextTexts.includes('行 12'), true)
  check('保留改动后 3 行上下文', contextTexts.includes('行 18'), true)
}

console.log(failures === 0 ? '\n全部通过' : `\n失败 ${failures} 项`)
process.exit(failures === 0 ? 0 : 1)
