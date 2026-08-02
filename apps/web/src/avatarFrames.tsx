// 头像框渲染层（仅组件导出，数据在 avatarFrameData.ts）。
//
// AvatarFrameLayer 契约：frame 为预设 id 时渲染星座 SVG/装饰；为非空图片 URL 时
// 渲染自定义图片；为空返回 null（调用方回退到等级框）。集成方负责把
// 'custom:<key>' 解析成图片 URL 后再传入。

import { findAvatarFrame, type AvatarFramePreset } from './avatarFrameData'

// 每个星座框的装饰点位（角度，0=顶部，顺时针）。不同星座点位/数量不同以增加辨识度。
const FRAME_DOTS: Record<string, number[]> = {
  'zodiac-aries': [0, 120, 240],
  'zodiac-taurus': [30, 150, 270],
  'zodiac-gemini': [0, 90, 180, 270],
  'zodiac-cancer': [45, 135, 225, 315],
  'zodiac-leo': [0, 72, 144, 216, 288],
  'zodiac-virgo': [20, 100, 180, 260, 340],
  'zodiac-libra': [0, 180],
  'zodiac-scorpio': [0, 60, 120, 180, 240, 300],
  'zodiac-sagittarius': [0, 130, 230],
  'zodiac-capricorn': [40, 160, 280],
  'zodiac-aquarius': [0, 90, 180, 270],
  'zodiac-pisces': [30, 150, 210, 330],
}

// zodiacFrameDecoration 渲染某星座预设的动态装饰环。
// 结构：流光渐变环（CSS conic + mask）+ 顶部符号徽标 + 环上闪烁星点。
// 每个星座通过 .af-<id> 类携带自己的配色变量（见 avatar-frames.css）。
function zodiacFrameDecoration(preset: AvatarFramePreset) {
  const dots = FRAME_DOTS[preset.id] ?? [0, 120, 240]
  return (
    <>
      <span className="af-ring" />
      <span className="af-halo" />
      <span className="af-stars">
        {dots.map((angle) => (
          <i key={angle} style={{ transform: `rotate(${angle}deg)` }} />
        ))}
      </span>
      <span className="af-badge">{preset.glyph}</span>
    </>
  )
}

export function AvatarFrameLayer({ frame, size = 'md' }: { frame: string; size?: 'sm' | 'md' | 'lg' }) {
  if (!frame) return null
  const preset = findAvatarFrame(frame)
  if (preset) {
    return (
      <span className={`avatar-frame-preset af-${preset.id} af-${size}`} aria-hidden="true">
        {zodiacFrameDecoration(preset)}
      </span>
    )
  }
  return (
    <span className={`avatar-frame-custom af-${size}`} aria-hidden="true">
      <img src={frame} alt="" />
    </span>
  )
}
