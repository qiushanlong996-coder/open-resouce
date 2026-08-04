// diffLines.js 的类型声明。实现写成 .js 以便校验脚本直接运行。

export type DiffKind = 'equal' | 'added' | 'removed'

export type DiffRow = {
  kind: DiffKind
  text: string
  // 行号：removed 只有左侧行号，added 只有右侧行号，equal 两侧都有。
  leftLine: number | null
  rightLine: number | null
}

export type DiffResult = {
  rows: DiffRow[]
  added: number
  removed: number
  // truncated 为真表示文档过长，已退化为整块替换而不是逐行对比。
  truncated: boolean
}

export type DiffChunk = { type: 'rows'; rows: DiffRow[] } | { type: 'gap'; lines: number }

export declare function diffLines(before: string, after: string): DiffResult
export declare function collapseDiffRows(rows: DiffRow[], context?: number): DiffChunk[]
