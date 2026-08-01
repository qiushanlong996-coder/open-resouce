import { useEffect, useMemo, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, FileText, Plus, Trash2 } from 'lucide-react'
import {
  ApiError,
  createAuthorProjectDocument,
  deleteAuthorProjectDocument,
  getAuthorProjectDocuments,
  moveAuthorProjectDocument,
  updateAuthorProjectDocument,
  type DocumentNode,
  type ProjectDocument,
} from './api/client'

// 项目文档树管理：新建、重命名、改写正文、调整层级与排序、删除。
// 对齐在线文档工具的知识库操作，一个项目可以有多层文档。

// slugFromTitle 根据标题推导默认 slug；中文标题无法直接转写时回退为时间戳。
function slugFromTitle(title: string) {
  const ascii = title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return ascii || `doc-${Date.now().toString(36)}`
}

function flattenTree(nodes: DocumentNode[], depth = 0): { node: DocumentNode; depth: number }[] {
  return nodes.flatMap((node) => [
    { node, depth },
    ...flattenTree(node.children ?? [], depth + 1),
  ])
}

export default function ProjectDocumentTree({
  projectID,
  onSelect,
  selectedDocumentID,
  refreshToken,
  showToast,
}: {
  projectID: string
  onSelect: (document: ProjectDocument | null) => void
  selectedDocumentID: string
  // refreshToken 变化时重拉列表，用于编辑器保存后同步最新正文，
  // 避免树里缓存的旧 markdown 在重新选中时覆盖编辑内容。
  refreshToken: number
  showToast: (message: string) => void
}) {
  const [documents, setDocuments] = useState<ProjectDocument[]>([])
  const [tree, setTree] = useState<DocumentNode[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})
  const [renamingID, setRenamingID] = useState('')
  const [renameDraft, setRenameDraft] = useState('')
  // 新建与删除均使用内联交互，不用浏览器原生 prompt/confirm：
  // 原生弹窗观感粗糙，且在部分浏览器上下文会被直接屏蔽。
  const [creatingParent, setCreatingParent] = useState<string | null | undefined>(undefined)
  const [createDraft, setCreateDraft] = useState('')
  const [pendingDeleteID, setPendingDeleteID] = useState('')
  const onSelectRef = useRef(onSelect)
  onSelectRef.current = onSelect

  const load = (selectID?: string) => {
    setLoading(true)
    getAuthorProjectDocuments(projectID)
      .then((response) => {
        setDocuments(response.data)
        setTree(response.tree)
        setError('')
        if (selectID) {
          const target = response.data.find((item) => item.id === selectID)
          if (target) onSelectRef.current(target)
        }
      })
      .catch((reason: unknown) => {
        setError(reason instanceof ApiError ? reason.message : '文档目录加载失败')
      })
      .finally(() => setLoading(false))
  }
  // load 每次渲染都会重建，用 ref 持有最新引用，避免它进入依赖数组后反复触发。
  const loadRef = useRef(load)
  loadRef.current = load

  useEffect(() => {
    setCollapsed({})
    onSelectRef.current(null)
    loadRef.current()
    // 切换项目时重新加载目录。
  }, [projectID])

  useEffect(() => {
    if (refreshToken === 0) return
    // 只刷新数据，不改变当前选中项。
    loadRef.current()
  }, [refreshToken])

  const documentsByID = useMemo(() => {
    const map = new Map<string, ProjectDocument>()
    documents.forEach((document) => map.set(document.id, document))
    return map
  }, [documents])

  const createDocument = (parentID: string | null, rawTitle: string) => {
    const trimmed = rawTitle.trim()
    if (!trimmed) {
      showToast('文档标题不能为空')
      return
    }
    setBusy(true)
    createAuthorProjectDocument(projectID, {
      parent_id: parentID, slug: slugFromTitle(trimmed), title: trimmed,
      markdown: `# ${trimmed}\n\n`,
    })
      .then((response) => {
        showToast('文档已创建')
        setCreatingParent(undefined)
        setCreateDraft('')
        if (parentID) setCollapsed((current) => ({ ...current, [parentID]: false }))
        load(response.data.id)
      })
      .catch((reason: unknown) => {
        showToast(reason instanceof ApiError ? reason.message : '文档创建失败')
      })
      .finally(() => setBusy(false))
  }

  const startCreate = (parentID: string | null) => {
    setCreatingParent(parentID)
    setCreateDraft('')
    if (parentID) setCollapsed((current) => ({ ...current, [parentID]: false }))
  }

  const submitRename = (document: ProjectDocument) => {
    const title = renameDraft.trim()
    setRenamingID('')
    if (!title || title === document.title) return
    setBusy(true)
    updateAuthorProjectDocument(projectID, document.id, {
      parent_id: document.parent_id, slug: document.slug, title, markdown: document.markdown,
    })
      .then(() => {
        showToast('文档已重命名')
        load()
      })
      .catch((reason: unknown) => {
        showToast(reason instanceof ApiError ? reason.message : '重命名失败')
      })
      .finally(() => setBusy(false))
  }

  const removeDocument = (document: ProjectDocument) => {
    setBusy(true)
    setPendingDeleteID('')
    deleteAuthorProjectDocument(projectID, document.id)
      .then(() => {
        showToast('文档已删除')
        if (document.id === selectedDocumentID) onSelectRef.current(null)
        load()
      })
      .catch((reason: unknown) => {
        showToast(reason instanceof ApiError ? reason.message : '删除失败')
      })
      .finally(() => setBusy(false))
  }

  // moveDocument 在同级内上下调整顺序：与相邻兄弟交换排序值。
  const moveDocument = (document: ProjectDocument, delta: number) => {
    const siblings = documents
      .filter((item) => (item.parent_id ?? '') === (document.parent_id ?? ''))
      .sort((left, right) => left.sort_order - right.sort_order || left.title.localeCompare(right.title, 'zh-CN'))
    const index = siblings.findIndex((item) => item.id === document.id)
    const target = siblings[index + delta]
    if (!target) return
    setBusy(true)
    Promise.all([
      moveAuthorProjectDocument(projectID, document.id, {
        parent_id: document.parent_id, sort_order: target.sort_order,
      }),
      moveAuthorProjectDocument(projectID, target.id, {
        parent_id: target.parent_id, sort_order: document.sort_order,
      }),
    ])
      .then(() => load())
      .catch((reason: unknown) => {
        showToast(reason instanceof ApiError ? reason.message : '排序调整失败')
      })
      .finally(() => setBusy(false))
  }

  // changeParent 调整层级：升级到上一层，或降级挂到前一个兄弟下。
  const changeParent = (document: ProjectDocument, direction: 'in' | 'out') => {
    let parentID: string | null = null
    if (direction === 'out') {
      const parent = document.parent_id ? documentsByID.get(document.parent_id) : undefined
      if (!document.parent_id) return
      parentID = parent?.parent_id ?? null
    } else {
      const siblings = documents
        .filter((item) => (item.parent_id ?? '') === (document.parent_id ?? ''))
        .sort((left, right) => left.sort_order - right.sort_order || left.title.localeCompare(right.title, 'zh-CN'))
      const index = siblings.findIndex((item) => item.id === document.id)
      const previous = siblings[index - 1]
      if (!previous) {
        showToast('上方没有可作为父级的文档')
        return
      }
      parentID = previous.id
    }
    setBusy(true)
    moveAuthorProjectDocument(projectID, document.id, { parent_id: parentID, sort_order: 0 })
      .then(() => {
        showToast('目录层级已调整')
        load()
      })
      .catch((reason: unknown) => {
        showToast(reason instanceof ApiError ? reason.message : '层级调整失败')
      })
      .finally(() => setBusy(false))
  }

  const rows = flattenTree(tree).filter(({ node }) => {
    // 父级折叠时隐藏其后代。
    let cursor = documentsByID.get(node.id)?.parent_id ?? null
    while (cursor) {
      if (collapsed[cursor]) return false
      cursor = documentsByID.get(cursor)?.parent_id ?? null
    }
    return true
  })

  // 内联新建输入行：parent 为 null 表示新建根文档。
  const renderCreateRow = (parent: string | null, depth: number) => (
    <div className="document-tree-row is-editing" style={{ paddingLeft: `${8 + depth * 16}px` }}>
      <span className="document-tree-toggle"><FileText size={13} /></span>
      <input
        autoFocus
        className="document-tree-rename"
        value={createDraft}
        placeholder={parent ? '子文档标题' : '文档标题'}
        onChange={(event) => setCreateDraft(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') createDocument(parent, createDraft)
          if (event.key === 'Escape') { setCreatingParent(undefined); setCreateDraft('') }
        }}
      />
      <span className="document-tree-actions is-visible">
        <button type="button" title="确认新建" disabled={busy} onClick={() => createDocument(parent, createDraft)}>✓</button>
        <button type="button" title="取消" onClick={() => { setCreatingParent(undefined); setCreateDraft('') }}>✕</button>
      </span>
    </div>
  )

  return (
    <section className="document-tree-panel">
      <div className="document-tree-head">
        <span className="meta-label">文档目录</span>
        <button type="button" disabled={busy} onClick={() => startCreate(null)}>
          <Plus size={13} /> 新建文档
        </button>
      </div>
      {error && <div className="auth-error">{error}</div>}
      {loading ? (
        <div className="document-tree-empty">正在加载文档目录…</div>
      ) : rows.length === 0 && creatingParent === undefined ? (
        <div className="document-tree-empty">
          还没有文档。新建第一篇后，阅读页会用文档目录替代项目单篇正文。
        </div>
      ) : (
        <div className="document-tree-list">
          {creatingParent === null && renderCreateRow(null, 0)}
          {rows.map(({ node, depth }) => {
            const document = documentsByID.get(node.id)
            if (!document) return null
            const hasChildren = (node.children ?? []).length > 0
            if (pendingDeleteID === node.id) {
              const childCount = documents.filter((item) => item.parent_id === node.id).length
              return (
                <div key={node.id} className="document-tree-row is-danger" style={{ paddingLeft: `${8 + depth * 16}px` }}>
                  <span className="document-tree-confirm">
                    {childCount > 0 ? `删除《${document.title}》及其 ${childCount} 篇子文档？` : `删除《${document.title}》？`}
                  </span>
                  <span className="document-tree-actions is-visible">
                    <button type="button" className="danger" disabled={busy} onClick={() => removeDocument(document)}>删除</button>
                    <button type="button" onClick={() => setPendingDeleteID('')}>取消</button>
                  </span>
                </div>
              )
            }
            return (
              <div key={node.id}>
                <div
                  className={`document-tree-row ${node.id === selectedDocumentID ? 'active' : ''}`}
                  style={{ paddingLeft: `${8 + depth * 16}px` }}
                >
                <button
                  className="document-tree-toggle"
                  type="button"
                  aria-label={collapsed[node.id] ? '展开子文档' : '折叠子文档'}
                  disabled={!hasChildren}
                  onClick={() => setCollapsed((current) => ({ ...current, [node.id]: !current[node.id] }))}
                >
                  {hasChildren
                    ? (collapsed[node.id] ? <ChevronRight size={13} /> : <ChevronDown size={13} />)
                    : <FileText size={13} />}
                </button>
                {renamingID === node.id ? (
                  <input
                    autoFocus
                    className="document-tree-rename"
                    value={renameDraft}
                    onChange={(event) => setRenameDraft(event.target.value)}
                    onBlur={() => submitRename(document)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') submitRename(document)
                      if (event.key === 'Escape') setRenamingID('')
                    }}
                  />
                ) : (
                  <button
                    className="document-tree-title"
                    type="button"
                    title={`${document.title}（${document.slug}）`}
                    onClick={() => onSelectRef.current(document)}
                    onDoubleClick={() => { setRenameDraft(document.title); setRenamingID(node.id) }}
                  >
                    {document.title}
                  </button>
                )}
                <span className="document-tree-actions">
                  <button type="button" title="上移" disabled={busy} onClick={() => moveDocument(document, -1)}>↑</button>
                  <button type="button" title="下移" disabled={busy} onClick={() => moveDocument(document, 1)}>↓</button>
                  <button type="button" title="降为上一篇的子文档" disabled={busy} onClick={() => changeParent(document, 'in')}>→</button>
                  <button type="button" title="升到上一层" disabled={busy || !document.parent_id} onClick={() => changeParent(document, 'out')}>←</button>
                  <button type="button" title="新建子文档" disabled={busy} onClick={() => startCreate(document.id)}><Plus size={12} /></button>
                  <button type="button" title="删除文档" disabled={busy} onClick={() => setPendingDeleteID(node.id)}><Trash2 size={12} /></button>
                </span>
                </div>
                {/* 当前行下方渲染子文档的内联新建输入。 */}
                {creatingParent === node.id && renderCreateRow(node.id, depth + 1)}
              </div>
            )
          })}
        </div>
      )}
      <p className="document-tree-tip">双击标题可重命名；→ 降级，← 升级，↑↓ 调整同级顺序。</p>
    </section>
  )
}
