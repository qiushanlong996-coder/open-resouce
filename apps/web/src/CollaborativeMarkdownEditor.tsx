import { useEffect, useRef, useState } from 'react'
import type { AuthUser } from './api/client'

type CollaborationStatus = 'connecting' | 'online' | 'saving' | 'saved' | 'offline'

type PresenceUser = {
  clientID: number
  id: string
  name: string
  color: string
}

type WireMessage = {
  type: string
  update?: string
  snapshot?: string
  markdown?: string
  revision?: number
  saved_at?: string
  client_id?: number
  user_id?: string
  display_name?: string
  message?: string
}

function bytesToBase64(bytes: Uint8Array) {
  let binary = ''
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000))
  }
  return window.btoa(binary)
}

function base64ToBytes(value: string) {
  const binary = window.atob(value)
  const result = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) result[index] = binary.charCodeAt(index)
  return result
}

function collaborationColor(value: string) {
  const colors = ['#0b69b7', '#9b3b72', '#19734a', '#a25513', '#6845a6', '#b83232', '#0d7f83']
  let hash = 0
  for (const character of value) hash = ((hash << 5) - hash + character.charCodeAt(0)) | 0
  return colors[Math.abs(hash) % colors.length]
}

export default function CollaborativeMarkdownEditor({
  slug,
  initialMarkdown,
  user,
  webSocketURL,
  onSaved,
}: {
  slug: string
  initialMarkdown: string
  user: AuthUser
  webSocketURL: string
  onSaved: (markdown: string) => void
}) {
  const rootRef = useRef<HTMLDivElement | null>(null)
  const [status, setStatus] = useState<CollaborationStatus>('connecting')
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [presence, setPresence] = useState<PresenceUser[]>([])
  const onSavedRef = useRef(onSaved)
  onSavedRef.current = onSaved

  useEffect(() => {
    let disposed = false
    let cleanupEditor = async () => {}
    const socket = new WebSocket(webSocketURL)

    socket.onopen = () => setStatus('connecting')
    socket.onerror = () => {
      if (!disposed) {
        setStatus('offline')
        setError('实时协作连接失败，请检查网络后重新进入编辑。')
      }
    }

    socket.onmessage = async (event) => {
      const message = JSON.parse(String(event.data)) as WireMessage
      if (message.type !== 'init') return
      socket.onmessage = null

      const [
        { Crepe },
        { collab, collabServiceCtx },
        Y,
        { Awareness, applyAwarenessUpdate, encodeAwarenessUpdate, removeAwarenessStates },
      ] = await Promise.all([
        import('@milkdown/crepe'),
        import('@milkdown/plugin-collab'),
        import('yjs'),
        import('y-protocols/awareness'),
      ])
      if (disposed || !rootRef.current) return

      const yDoc = new Y.Doc()
      const awareness = new Awareness(yDoc)
      const remoteOrigin = { remote: true }
      const color = collaborationColor(user.id)
      const currentMarkdown = { value: message.markdown || initialMarkdown }
      let saveTimer: number | undefined
      let crepe: InstanceType<typeof Crepe> | null = null

      if (message.snapshot) Y.applyUpdate(yDoc, base64ToBytes(message.snapshot), remoteOrigin)
      setRevision(message.revision ?? 0)

      const updatePresence = () => {
        const next: PresenceUser[] = []
        awareness.getStates().forEach((state, clientID) => {
          const stateUser = state.user as { id?: string; name?: string; color?: string } | undefined
          if (stateUser?.id && stateUser.name && stateUser.color) {
            next.push({ clientID, id: stateUser.id, name: stateUser.name, color: stateUser.color })
          }
        })
        next.sort((left, right) => left.name.localeCompare(right.name, 'zh-CN'))
        setPresence(next)
      }

      const send = (payload: WireMessage) => {
        if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify(payload))
      }
      const sendAwareness = (clientIDs: number[]) => {
        if (clientIDs.length === 0) return
        send({
          type: 'awareness',
          update: bytesToBase64(encodeAwarenessUpdate(awareness, clientIDs)),
          client_id: yDoc.clientID,
        })
      }
      const scheduleSave = () => {
        window.clearTimeout(saveTimer)
        setStatus('saving')
        saveTimer = window.setTimeout(() => {
          if (!crepe || socket.readyState !== WebSocket.OPEN) return
          currentMarkdown.value = crepe.getMarkdown().slice(0, 50000)
          send({
            type: 'snapshot',
            snapshot: bytesToBase64(Y.encodeStateAsUpdate(yDoc)),
            markdown: currentMarkdown.value,
          })
        }, 1000)
      }

      yDoc.on('update', (update: Uint8Array, origin: unknown) => {
        if (origin !== remoteOrigin) send({ type: 'update', update: bytesToBase64(update) })
        window.setTimeout(scheduleSave, 0)
      })
      awareness.on('update', (
        changes: { added: number[]; updated: number[]; removed: number[] },
        origin: unknown,
      ) => {
        updatePresence()
        if (origin !== remoteOrigin) {
          sendAwareness([...changes.added, ...changes.updated, ...changes.removed])
        }
      })

      crepe = new Crepe({
        root: rootRef.current,
        defaultValue: '',
        features: { [Crepe.Feature.TopBar]: true },
      })
      crepe.editor.use(collab)
      crepe.on((listener) => {
        listener.markdownUpdated((_ctx, markdown) => {
          currentMarkdown.value = markdown.slice(0, 50000)
          scheduleSave()
        })
      })
      await crepe.create()
      if (disposed) {
        await crepe.destroy()
        return
      }

      crepe.editor.action((ctx) => {
        const service = ctx.get(collabServiceCtx)
        service.bindDoc(yDoc).setAwareness(awareness).mergeOptions({
          yCursorOpts: {
            cursorBuilder: (remoteUser: { name?: string; color?: string }) => {
              const cursor = window.document.createElement('span')
              cursor.className = 'collaboration-remote-cursor'
              cursor.style.borderColor = remoteUser.color ?? '#0b69b7'
              const label = window.document.createElement('span')
              label.style.backgroundColor = remoteUser.color ?? '#0b69b7'
              label.textContent = remoteUser.name ?? '协作者'
              cursor.append(label)
              return cursor
            },
          },
        })
        if (!message.snapshot) service.applyTemplate(message.markdown || initialMarkdown)
        service.connect()
      })

      awareness.setLocalStateField('user', { id: user.id, name: user.display_name, color })
      send({ type: 'presence', client_id: yDoc.clientID })
      sendAwareness([yDoc.clientID])
      setStatus('online')
      updatePresence()

      socket.onmessage = (nextEvent) => {
        const next = JSON.parse(String(nextEvent.data)) as WireMessage
        if (next.type === 'update' && next.update) {
          Y.applyUpdate(yDoc, base64ToBytes(next.update), remoteOrigin)
        } else if (next.type === 'awareness' && next.update) {
          applyAwarenessUpdate(awareness, base64ToBytes(next.update), remoteOrigin)
        } else if (next.type === 'presence-request') {
          sendAwareness([yDoc.clientID])
        } else if (next.type === 'presence-left' && next.client_id) {
          removeAwarenessStates(awareness, [next.client_id], remoteOrigin)
        } else if (next.type === 'saved') {
          setRevision((current) => next.revision ?? current)
          setStatus('saved')
          onSavedRef.current(currentMarkdown.value)
        } else if (next.type === 'error') {
          setError(next.message ?? '协作服务发生错误。')
          setStatus('offline')
        }
      }

      cleanupEditor = async () => {
        window.clearTimeout(saveTimer)
        awareness.setLocalState(null)
        awareness.destroy()
        yDoc.destroy()
        await crepe?.destroy()
      }
    }

    socket.onclose = () => {
      if (!disposed) setStatus('offline')
    }

    return () => {
      disposed = true
      socket.close(1000, 'editor closed')
      void cleanupEditor()
    }
  }, [initialMarkdown, slug, user.display_name, user.id, webSocketURL])

  const statusText = {
    connecting: '正在连接协作空间…',
    online: '实时协作已连接',
    saving: '正在保存所有人的更改…',
    saved: `已保存 · 修订 ${revision}`,
    offline: '协作连接已断开',
  }[status]

  return (
    <section className="collaborative-editor">
      <div className="collaboration-statusbar">
        <span className={`collaboration-state ${status}`}><i />{statusText}</span>
        <div className="collaboration-presence" aria-label={`${presence.length} 人在线`}>
          {presence.slice(0, 6).map((person) => (
            <span key={person.clientID} title={person.name} style={{ backgroundColor: person.color }}>
              {person.name.slice(0, 1)}
            </span>
          ))}
          <small>{presence.length} 人在线</small>
        </div>
      </div>
      {error && <div className="auth-error">{error}</div>}
      <div className="collaboration-editor-root" ref={rootRef} />
    </section>
  )
}
