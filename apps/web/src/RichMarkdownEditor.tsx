import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react'
import type { Crepe as CrepeInstance } from '@milkdown/crepe'
import '@milkdown/crepe/theme/common/style.css'
import '@milkdown/crepe/theme/frame.css'

export type RichMarkdownEditorHandle = {
  insertMarkdown: (markdown: string) => void
}

type InsertMarkdown = (markdown: string) => void

function encodedObjectKey(value: string) {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach((byte) => { binary += String.fromCharCode(byte) })
  return window.btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

type UploadKind = 'image' | 'document' | 'code'

// 依据 MIME / 扩展名推断文件类别，与后端 validObjectUpload 的允许项保持一致。
function inferUploadKind(file: File): UploadKind | null {
  if (file.type.startsWith('image/')) return 'image'
  const name = file.name.toLowerCase()
  if (/\.(jpg|jpeg|png|webp|gif)$/.test(name)) return 'image'
  if (file.type === 'application/pdf' || /\.(pdf|md|txt)$/.test(name)) return 'document'
  if (/\.(zip|gz|tgz|tar)$/.test(name)) return 'code'
  return null
}

const RichMarkdownEditor = forwardRef<RichMarkdownEditorHandle, {
  value: string
  documentKey: string
  onChange: (markdown: string) => void
  onUploadImage: (file: File) => Promise<string>
  onUploadFile?: (file: File) => Promise<string>
  onNotify?: (message: string) => void
}>(function RichMarkdownEditor({ value, documentKey, onChange, onUploadImage, onUploadFile, onNotify }, ref) {
  const rootRef = useRef<HTMLDivElement | null>(null)
  const editorRef = useRef<CrepeInstance | null>(null)
  const insertRef = useRef<InsertMarkdown | null>(null)
  const onChangeRef = useRef(onChange)
  const onUploadImageRef = useRef(onUploadImage)
  const onUploadFileRef = useRef(onUploadFile)
  const onNotifyRef = useRef(onNotify)
  const initialValueRef = useRef(value)
  const [loading, setLoading] = useState(true)

  onChangeRef.current = onChange
  onUploadImageRef.current = onUploadImage
  onUploadFileRef.current = onUploadFile
  onNotifyRef.current = onNotify
  initialValueRef.current = value

  useImperativeHandle(ref, () => ({
    insertMarkdown(markdown: string) {
      insertRef.current?.(markdown)
    },
  }), [])

  useEffect(() => {
    let cancelled = false
    let removeListeners = () => {}
    setLoading(true)

    Promise.all([
      import('@milkdown/crepe'),
      import('@milkdown/kit/utils'),
    ]).then(async ([{ Crepe }, { insert }]) => {
      if (!rootRef.current || cancelled) return

      const crepe = new Crepe({
        root: rootRef.current,
        defaultValue: initialValueRef.current || '# 项目介绍\n\n从这里开始编写项目文档。',
        features: {
          [Crepe.Feature.TopBar]: true,
        },
        featureConfigs: {
          [Crepe.Feature.ImageBlock]: {
            blockUploadButton: '上传图片',
            inlineUploadButton: '上传图片',
            blockConfirmButton: '确认',
            inlineConfirmButton: '确认',
            blockUploadPlaceholderText: '上传图片或粘贴图片地址…',
            inlineUploadPlaceholderText: '上传图片或粘贴图片地址…',
            blockCaptionPlaceholderText: '图片说明',
            onUpload: (file: File) => onUploadImageRef.current(file),
            proxyDomURL: (url: string) => {
              if (!url.startsWith('oss://')) return url
              return `/api/v1/files/author-asset?key=${encodeURIComponent(encodedObjectKey(url.slice(6)))}`
            },
          },
        },
      })
      crepe.on((listener) => {
        listener.markdownUpdated((_ctx, markdown) => {
          if (!cancelled) onChangeRef.current(markdown)
        })
      })
      await crepe.create()

      if (cancelled) {
        await crepe.destroy()
        return
      }
      editorRef.current = crepe
      const insertMarkdown: InsertMarkdown = (markdown: string) => {
        crepe.editor.action(insert(markdown))
      }
      insertRef.current = insertMarkdown

      // 粘贴 / 拖拽文件时即时上传并原地插入（图片内联渲染，其它文件插入下载链接）。
      const uploadFiles = async (files: File[]) => {
        for (const file of files) {
          const kind = inferUploadKind(file)
          if (kind === null) {
            onNotifyRef.current?.('暂不支持该文件类型（仅支持 图片 / pdf / md / txt / zip / tar.gz）')
            continue
          }
          if (kind !== 'image' && !onUploadFileRef.current) {
            onNotifyRef.current?.('暂不支持上传附件')
            continue
          }
          onNotifyRef.current?.('上传中…')
          try {
            if (kind === 'image') {
              const url = await onUploadImageRef.current(file)
              insertMarkdown(`\n![${file.name}](${url})\n`)
            } else {
              const url = await onUploadFileRef.current!(file)
              insertMarkdown(`\n[📎 ${file.name}](${url})\n`)
            }
          } catch (reason) {
            const message = reason instanceof Error ? reason.message : '文件上传失败'
            onNotifyRef.current?.(message)
          }
        }
      }
      const handlePaste = (event: ClipboardEvent) => {
        const files = Array.from(event.clipboardData?.files ?? [])
        if (files.length === 0) return
        event.preventDefault()
        void uploadFiles(files)
      }
      const handleDrop = (event: DragEvent) => {
        const files = Array.from(event.dataTransfer?.files ?? [])
        if (files.length === 0) return
        event.preventDefault()
        void uploadFiles(files)
      }
      const dropTarget = rootRef.current
      dropTarget.addEventListener('paste', handlePaste)
      dropTarget.addEventListener('drop', handleDrop)
      removeListeners = () => {
        dropTarget.removeEventListener('paste', handlePaste)
        dropTarget.removeEventListener('drop', handleDrop)
      }

      setLoading(false)
    }).catch(() => {
      if (!cancelled) setLoading(false)
    })

    return () => {
      cancelled = true
      removeListeners()
      insertRef.current = null
      const editor = editorRef.current
      editorRef.current = null
      if (editor) void editor.destroy()
    }
  }, [documentKey])

  return (
    <div className="rich-markdown-editor">
      {loading && <div className="rich-editor-loading">正在加载富文本编辑器…</div>}
      <div ref={rootRef} />
    </div>
  )
})

export default RichMarkdownEditor
