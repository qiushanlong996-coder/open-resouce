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

const RichMarkdownEditor = forwardRef<RichMarkdownEditorHandle, {
  value: string
  documentKey: string
  onChange: (markdown: string) => void
  onUploadImage: (file: File) => Promise<string>
}>(function RichMarkdownEditor({ value, documentKey, onChange, onUploadImage }, ref) {
  const rootRef = useRef<HTMLDivElement | null>(null)
  const editorRef = useRef<CrepeInstance | null>(null)
  const insertRef = useRef<InsertMarkdown | null>(null)
  const onChangeRef = useRef(onChange)
  const onUploadImageRef = useRef(onUploadImage)
  const initialValueRef = useRef(value)
  const [loading, setLoading] = useState(true)

  onChangeRef.current = onChange
  onUploadImageRef.current = onUploadImage
  initialValueRef.current = value

  useImperativeHandle(ref, () => ({
    insertMarkdown(markdown: string) {
      insertRef.current?.(markdown)
    },
  }), [])

  useEffect(() => {
    let cancelled = false
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
      insertRef.current = (markdown: string) => {
        crepe.editor.action(insert(markdown))
      }
      setLoading(false)
    }).catch(() => {
      if (!cancelled) setLoading(false)
    })

    return () => {
      cancelled = true
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
