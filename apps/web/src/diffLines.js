// 逐行对比历史版本与当前正文。
//
// 走经典 LCS 动态规划：对 Markdown 这种以行为单位的内容，行级对比就足够直观，
// 而且结果稳定——不会像字符级 diff 那样把整段重排渲染成一片红绿噪声。
//
// 为避免长文档把主线程卡住，超过 maxDiffLines 行时退化为整块替换：
// 一次 LCS 是 O(m×n)，两篇 4000 行的文档就是一千六百万个格子。
//
// 实现写成 .js（类型见 diffLines.d.ts），以便校验脚本用 node 直接运行。
// 回滚是有破坏性的操作，作者要靠这里的对比结果决定要不要回滚，
// 所以这段逻辑必须有可执行的断言守住。

const maxDiffLines = 2000

export function diffLines(before, after) {
  const left = before.length === 0 ? [] : before.split('\n')
  const right = after.length === 0 ? [] : after.split('\n')

  if (left.length > maxDiffLines || right.length > maxDiffLines) {
    const rows = [
      ...left.map((text, index) => ({ kind: 'removed', text, leftLine: index + 1, rightLine: null })),
      ...right.map((text, index) => ({ kind: 'added', text, leftLine: null, rightLine: index + 1 })),
    ]
    return { rows, added: right.length, removed: left.length, truncated: true }
  }

  // lengths[i][j] 是 left[i:] 与 right[j:] 的最长公共子序列长度。
  const lengths = Array.from({ length: left.length + 1 }, () => new Array(right.length + 1).fill(0))
  for (let i = left.length - 1; i >= 0; i -= 1) {
    for (let j = right.length - 1; j >= 0; j -= 1) {
      lengths[i][j] = left[i] === right[j]
        ? lengths[i + 1][j + 1] + 1
        : Math.max(lengths[i + 1][j], lengths[i][j + 1])
    }
  }

  const rows = []
  let added = 0
  let removed = 0
  let i = 0
  let j = 0
  while (i < left.length && j < right.length) {
    if (left[i] === right[j]) {
      rows.push({ kind: 'equal', text: left[i], leftLine: i + 1, rightLine: j + 1 })
      i += 1
      j += 1
      continue
    }
    // 相等长度时先输出删除行，让「旧 → 新」的阅读顺序保持一致。
    if (lengths[i + 1][j] >= lengths[i][j + 1]) {
      rows.push({ kind: 'removed', text: left[i], leftLine: i + 1, rightLine: null })
      removed += 1
      i += 1
    } else {
      rows.push({ kind: 'added', text: right[j], leftLine: null, rightLine: j + 1 })
      added += 1
      j += 1
    }
  }
  while (i < left.length) {
    rows.push({ kind: 'removed', text: left[i], leftLine: i + 1, rightLine: null })
    removed += 1
    i += 1
  }
  while (j < right.length) {
    rows.push({ kind: 'added', text: right[j], leftLine: null, rightLine: j + 1 })
    added += 1
    j += 1
  }
  return { rows, added, removed, truncated: false }
}

// collapseDiffRows 折叠大段未改动内容，只在改动前后各留 context 行。
// 返回的 gap 表示这里省略了多少行，供 UI 渲染「省略 N 行未改动内容」。
export function collapseDiffRows(rows, context = 3) {
  const keep = new Array(rows.length).fill(false)
  rows.forEach((row, index) => {
    if (row.kind === 'equal') return
    for (let offset = -context; offset <= context; offset += 1) {
      const target = index + offset
      if (target >= 0 && target < rows.length) keep[target] = true
    }
  })
  const chunks = []
  let cursor = 0
  while (cursor < rows.length) {
    if (keep[cursor]) {
      const start = cursor
      while (cursor < rows.length && keep[cursor]) cursor += 1
      chunks.push({ type: 'rows', rows: rows.slice(start, cursor) })
      continue
    }
    const start = cursor
    while (cursor < rows.length && !keep[cursor]) cursor += 1
    chunks.push({ type: 'gap', lines: cursor - start })
  }
  return chunks
}
