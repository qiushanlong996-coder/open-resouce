// 公开用户主页弹窗：展示作者昵称、等级、注册时间、统计与已发布项目。
// 点击某个项目通过 onOpenProject(slug) 复用 App 既有的打开项目路径。

import { useEffect, useState } from 'react'
import { CalendarDays, Download, Eye, FolderGit2, X } from 'lucide-react'
import { LevelAvatar, LevelBadge } from './LevelAvatar'
import { ApiError, getUserProfile, type PublicUserProfile } from './api/client'
import './user-profile.css'

const compact = new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 })

function formatJoined(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'long' }).format(date)
}

export function UserProfile({
  userId,
  onClose,
  onOpenProject,
}: {
  userId: string
  onClose: () => void
  onOpenProject: (slug: string) => void
}) {
  const [profile, setProfile] = useState<PublicUserProfile | null>(null)
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [errorMessage, setErrorMessage] = useState('该用户主页加载失败。')

  useEffect(() => {
    const controller = new AbortController()
    setState('loading')
    setProfile(null)
    getUserProfile(userId, controller.signal)
      .then((response) => {
        setProfile(response.data)
        setState('ready')
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setErrorMessage(error instanceof ApiError && error.status === 404 ? '该用户不存在。' : '该用户主页加载失败。')
        setState('error')
      })
    return () => controller.abort()
  }, [userId])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const joinedLabel = profile ? formatJoined(profile.joined_at) : ''

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="user-profile-modal"
        role="dialog"
        aria-modal="true"
        aria-label="用户主页"
        onClick={(event) => event.stopPropagation()}
      >
        <button className="modal-close" title="关闭" aria-label="关闭" onClick={onClose}><X size={18} /></button>

        {state === 'loading' && <div className="user-profile-state">正在加载用户主页…</div>}
        {state === 'error' && <div className="user-profile-state">{errorMessage}</div>}

        {state === 'ready' && profile && (
          <>
            <header className="user-profile-head">
              <LevelAvatar
                level={profile.level}
                initials={profile.display_name.slice(0, 1)}
                size="lg"
                name={profile.display_name}
                frame={profile.avatar_frame ?? ''}
              />
              <div className="user-profile-identity">
                <div className="user-profile-name-row">
                  <h2>{profile.display_name}</h2>
                  <LevelBadge level={profile.level} />
                </div>
                {joinedLabel && (
                  <span className="user-profile-joined"><CalendarDays size={13} /> 加入于 {joinedLabel}</span>
                )}
              </div>
            </header>

            <div className="user-profile-body">
              <div className="user-profile-stats">
                <div className="user-profile-stat">
                  <strong><FolderGit2 size={17} /> {profile.stats.projects_count}</strong>
                  <span>已发布项目</span>
                </div>
                <div className="user-profile-stat">
                  <strong><Eye size={17} /> {compact.format(profile.stats.total_views)}</strong>
                  <span>总浏览</span>
                </div>
                <div className="user-profile-stat">
                  <strong><Download size={17} /> {compact.format(profile.stats.total_downloads)}</strong>
                  <span>总下载</span>
                </div>
              </div>

              <h3 className="user-profile-section-title">已发布项目</h3>
              {profile.projects.length === 0 ? (
                <div className="user-profile-state">这位作者还没有已发布的项目。</div>
              ) : (
                <div className="user-profile-projects">
                  {profile.projects.map((project) => (
                    <button
                      key={project.id}
                      type="button"
                      className="user-profile-project"
                      onClick={() => onOpenProject(project.slug)}
                    >
                      <strong>{project.name}</strong>
                      <p>{project.summary}</p>
                      <div className="user-profile-project-meta">
                        <span><Eye size={11} /> {compact.format(project.metrics.views ?? 0)}</span>
                        <span><Download size={11} /> {compact.format(project.metrics.downloads)}</span>
                        <span>{project.category}</span>
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
