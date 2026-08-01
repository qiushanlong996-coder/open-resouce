// 用户等级头像框与等级徽标。
//
// 等级 1-6：头像框随等级变化，越高越高级（高等级带纯 CSS 动画，见 App.css）。
// 6 级为最高级，管理员由服务端固定为 6 级。等级 <1 或缺失时按 1 级处理。

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

// LevelAvatar 渲染带等级头像框的头像。frame 为纯装饰，用 aria-hidden 隐藏。
export function LevelAvatar({
  level,
  initials,
  size = 'md',
  name,
}: {
  level?: number
  initials: string
  size?: 'sm' | 'md' | 'lg'
  name?: string
}) {
  const lvl = normalizeLevel(level)
  const title = name ? `${name} · Lv.${lvl} ${levelName(lvl)}` : `Lv.${lvl} ${levelName(lvl)}`
  return (
    <span className={`level-avatar size-${size} lvl-${lvl}`} data-level={lvl} title={title}>
      <span className="level-avatar-frame" aria-hidden="true" />
      <span className="level-avatar-inner">{initials}</span>
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
