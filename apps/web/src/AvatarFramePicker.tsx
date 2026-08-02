// 头像框选择器弹窗：从账号菜单「头像框」打开（登录后可用）。
// 提供 12 星座预设、默认（等级框）与自定义上传三类选择；选中即持久化并即时更新当前用户。

import { useState } from 'react'
import { X, Upload } from 'lucide-react'
import { AVATAR_FRAMES } from './avatarFrameData'
import { LevelAvatar } from './LevelAvatar'
import { ApiError, setAvatarFrame, uploadProjectFile, type AuthUser } from './api/client'
import './avatar-frame-picker.css'

export function AvatarFramePicker({
  currentUser,
  onClose,
  onChanged,
}: {
  currentUser: AuthUser
  onClose: () => void
  onChanged: (user: AuthUser) => void
}) {
  const [saving, setSaving] = useState<string | null>(null)
  const [error, setError] = useState('')
  const initials = currentUser.display_name.slice(0, 1)
  const selected = currentUser.avatar_frame

  const apply = async (frame: string) => {
    if (saving) return
    setSaving(frame || 'default')
    setError('')
    try {
      const response = await setAvatarFrame(frame)
      onChanged(response.data)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '头像框更新失败，请稍后重试。')
    } finally {
      setSaving(null)
    }
  }

  const onUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    if (saving) return
    setSaving('upload')
    setError('')
    try {
      const key = await uploadProjectFile(file, 'image')
      const response = await setAvatarFrame('custom:' + key)
      onChanged(response.data)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '自定义头像框上传失败，请稍后重试。')
    } finally {
      setSaving(null)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="avatar-frame-picker"
        role="dialog"
        aria-modal="true"
        aria-label="选择头像框"
        onClick={(event) => event.stopPropagation()}
      >
        <button className="modal-close" title="关闭" aria-label="关闭" onClick={onClose}><X size={18} /></button>
        <h2 className="avatar-frame-picker-title">头像框</h2>
        <p className="avatar-frame-picker-hint">选择一个预设星座框，或上传自定义图片作为头像框。</p>
        {error && <div className="avatar-frame-picker-error" role="alert">{error}</div>}
        <div className="avatar-frame-grid">
          <button
            type="button"
            className={`avatar-frame-option ${selected === '' ? 'selected' : ''}`}
            disabled={saving !== null}
            onClick={() => void apply('')}
          >
            <LevelAvatar level={currentUser.level} initials={initials} size="lg" name={currentUser.display_name} frame="" />
            <span>默认（等级框）</span>
          </button>
          {AVATAR_FRAMES.map((preset) => (
            <button
              key={preset.id}
              type="button"
              className={`avatar-frame-option ${selected === preset.id ? 'selected' : ''}`}
              disabled={saving !== null}
              onClick={() => void apply(preset.id)}
            >
              <LevelAvatar level={currentUser.level} initials={initials} size="lg" name={preset.label} frame={preset.id} />
              <span>{preset.label}</span>
            </button>
          ))}
        </div>
        <label className={`avatar-frame-upload ${saving === 'upload' ? 'busy' : ''}`}>
          <Upload size={15} /> {saving === 'upload' ? '上传中…' : '上传自定义'}
          <input type="file" accept="image/*" disabled={saving !== null} onChange={(event) => void onUpload(event)} />
        </label>
      </div>
    </div>
  )
}
