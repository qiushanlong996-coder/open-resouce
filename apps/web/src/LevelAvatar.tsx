// 用户等级头像框与等级徽标。
//
// 等级 1-6：头像框随等级变化，越高越高级（高等级带纯 CSS 动画，见 App.css）。
// 6 级为最高级，管理员由服务端固定为 6 级。等级 <1 或缺失时按 1 级处理。

import { isPresetFrameId } from './avatarFrameData'
import { AvatarFrameLayer } from './avatarFrames'
import { avatarFrameImageURL, userImageURL } from './api/client'

const MAX_LEVEL = 6

// 各等级名称，索引即等级（0 位占位）。
const LEVEL_NAMES = ['', '新手', '进阶', '资深', '专家', '大师', '传奇'] as const

function normalizeLevel(level?: number): number {
  if (!level || level < 1) return 1
  if (level > MAX_LEVEL) return MAX_LEVEL
  return Math.floor(level)
}

function levelName(level?: number): string {
  return LEVEL_NAMES[normalizeLevel(level)]
}

// resolveFrameLayer 把存储值解析成可渲染的 AvatarFrameLayer：
// 空 → null（回退等级框）；预设 id → 原样传入；custom:<key> → 解析为图片 URL。
function resolveFrameLayer(frame: string, size: 'sm' | 'md' | 'lg') {
  if (!frame) return null
  if (isPresetFrameId(frame)) return <AvatarFrameLayer frame={frame} size={size} />
  if (frame.startsWith('custom:')) {
    return <AvatarFrameLayer frame={avatarFrameImageURL(frame.slice('custom:'.length))} size={size} />
  }
  return null
}

// LevelAvatar 渲染带头像框的头像。frame 为纯装饰，用 aria-hidden 隐藏。
// 传入 frame 时（预设或自定义）用其替换默认等级框；否则回退到随等级变化的 .level-avatar-frame。
export function LevelAvatar({
  level,
  initials,
  size = 'md',
  name,
  avatar = '',
  frame = '',
}: {
  level?: number
  initials: string
  size?: 'sm' | 'md' | 'lg'
  name?: string
  avatar?: string
  frame?: string
}) {
  const lvl = normalizeLevel(level)
  const title = name ? `${name} · Lv.${lvl} ${levelName(lvl)}` : `Lv.${lvl} ${levelName(lvl)}`
  const frameLayer = resolveFrameLayer(frame, size)
  return (
    <span className={`level-avatar size-${size} lvl-${lvl}`} data-level={lvl} title={title}>
      {frameLayer ?? <span className="level-avatar-frame" aria-hidden="true" />}
      <span className="level-avatar-inner">
        {avatar ? <img className="level-avatar-image" src={userImageURL(avatar)} alt="" /> : initials}
      </span>
    </span>
  )
}

// LevelBadge 是名字旁的 Lv.N 徽标；6 级用 legendary 特殊样式。
export function LevelBadge({ level }: { level?: number }) {
  const lvl = normalizeLevel(level)
  return (
    <span className={`level-badge lvl-${lvl}`} title={`Lv.${lvl} ${levelName(lvl)}`}>
      Lv.{lvl}
    </span>
  )
}
