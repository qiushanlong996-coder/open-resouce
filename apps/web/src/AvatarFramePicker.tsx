// 头像框选择器弹窗：从账号菜单「头像框」打开（登录后可用）。
// 提供 12 星座预设、默认（等级框）与自定义上传三类选择；选中即持久化并即时更新当前用户。

import { useState } from 'react'
import { ImageUp, Trash2, Upload, X } from 'lucide-react'
import { AVATAR_FRAMES } from './avatarFrameData'
import { LevelAvatar } from './LevelAvatar'
import { ApiError, setAvatar, setAvatarFrame, uploadProjectFile, type AuthUser } from './api/client'
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

  const validateImage = (file: File) => {
    if (!file.type.startsWith('image/')) return '请选择 JPG、PNG、WebP 或 GIF 图片。'
    if (file.size > 5 * 1024 * 1024) return '图片不能超过 5 MB。'
    return ''
  }

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

  const onAvatarUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file || saving) return
    const validationError = validateImage(file)
    if (validationError) {
      setError(validationError)
      return
    }
    setSaving('avatar-upload')
    setError('')
    try {
      const key = await uploadProjectFile(file, 'image')
      const response = await setAvatar(key)
      onChanged(response.data)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '头像上传失败，请稍后重试。')
    } finally {
      setSaving(null)
    }
  }

  const removeAvatar = async () => {
    if (saving) return
    setSaving('avatar-remove')
    setError('')
    try {
      const response = await setAvatar('')
      onChanged(response.data)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '头像移除失败，请稍后重试。')
    } finally {
      setSaving(null)
    }
  }

  const onFrameUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file || saving) return
    const validationError = validateImage(file)
    if (validationError) {
      setError(validationError)
      return
    }
    setSaving('frame-upload')
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
        aria-label="更换头像与头像框"
        onClick={(event) => event.stopPropagation()}
      >
        <button className="modal-close" title="关闭" aria-label="关闭" onClick={onClose}><X size={18} /></button>
        <h2 className="avatar-frame-picker-title">头像与头像框</h2>
        <p className="avatar-frame-picker-hint">上传你的头像，再选择一款装饰头像框。头像会自动居中裁切。</p>
        {error && <div className="avatar-frame-picker-error" role="alert">{error}</div>}
        <section className="avatar-picker-section" aria-labelledby="avatar-picker-heading">
          <div className="avatar-picker-section-head">
            <div>
              <h3 id="avatar-picker-heading">头像</h3>
              <p>支持 JPG、PNG、WebP、GIF，最大 5 MB。</p>
            </div>
            <LevelAvatar level={currentUser.level} initials={initials} size="lg" name={currentUser.display_name} avatar={currentUser.avatar} frame={currentUser.avatar_frame} />
          </div>
          <div className="avatar-picker-actions">
            <label className={`avatar-frame-upload ${saving === 'avatar-upload' ? 'busy' : ''}`}>
              <ImageUp size={16} /> {saving === 'avatar-upload' ? '上传中…' : currentUser.avatar ? '更换头像' : '上传头像'}
              <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" disabled={saving !== null} onChange={(event) => void onAvatarUpload(event)} />
            </label>
            {currentUser.avatar && (
              <button className="avatar-reset-button" type="button" disabled={saving !== null} onClick={() => void removeAvatar()}>
                <Trash2 size={15} /> {saving === 'avatar-remove' ? '移除中…' : '恢复默认'}
              </button>
            )}
          </div>
        </section>
        <section className="avatar-picker-section" aria-labelledby="avatar-frame-heading">
          <div className="avatar-picker-section-head compact">
            <div>
              <h3 id="avatar-frame-heading">头像框</h3>
              <p>装饰层不会覆盖或修改你的原头像。</p>
            </div>
          </div>
        <div className="avatar-frame-grid">
          <button
            type="button"
            className={`avatar-frame-option ${selected === '' ? 'selected' : ''}`}
            disabled={saving !== null}
            onClick={() => void apply('')}
          >
            <LevelAvatar level={currentUser.level} initials={initials} size="lg" name={currentUser.display_name} avatar={currentUser.avatar} frame="" />
            <span>默认（等级框）</span>
          </button>
          {AVATAR_FRAMES.slice(0, 1).map((preset) => (
            <button
              key={preset.id}
              type="button"
              className={`avatar-frame-option ${selected === preset.id ? 'selected' : ''}`}
              disabled={saving !== null}
              onClick={() => void apply(preset.id)}
            >
              <LevelAvatar level={currentUser.level} initials={initials} size="lg" name={preset.label} avatar={currentUser.avatar} frame={preset.id} />
              <span>花语</span>
            </button>
          ))}
        </div>
        <label className={`avatar-frame-upload secondary ${saving === 'frame-upload' ? 'busy' : ''}`}>
          <Upload size={15} /> {saving === 'frame-upload' ? '上传中…' : '上传自定义头像框'}
          <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" disabled={saving !== null} onChange={(event) => void onFrameUpload(event)} />
        </label>
        </section>
      </div>
    </div>
  )
}
