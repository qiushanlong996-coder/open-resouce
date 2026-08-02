// 头像框渲染层（仅组件导出，数据在 avatarFrameData.ts）。
//
// AvatarFrameLayer 契约：frame 为预设 id 时渲染星座 SVG；为非空图片 URL 时渲染
// 自定义图片；为空返回 null（调用方回退到等级框）。集成方负责把 'custom:<key>'
// 解析成图片 URL 后再传入。

import { findAvatarFrame, type AvatarFramePreset } from './avatarFrameData'

// zodiacFrameSvg 渲染某星座预设的动态 SVG 框。
// 占位实现，后续替换为逐星座的精细主题与动画（旋转/流光/星点）。
function zodiacFrameSvg(preset: AvatarFramePreset) {
  return (
    <svg className="avatar-frame-svg" viewBox="0 0 100 100" role="img" aria-label={preset.label}>
      <circle className="avatar-frame-ring" cx="50" cy="50" r="47" fill="none" strokeWidth="4" />
      <text className="avatar-frame-glyph" x="50" y="9" textAnchor="middle" dominantBaseline="middle">{preset.glyph}</text>
    </svg>
  )
}

export function AvatarFrameLayer({ frame, size = 'md' }: { frame: string; size?: 'sm' | 'md' | 'lg' }) {
  if (!frame) return null
  const preset = findAvatarFrame(frame)
  if (preset) {
    return (
      <span className={`avatar-frame-preset af-${preset.id} af-${size}`} aria-hidden="true">
        {zodiacFrameSvg(preset)}
      </span>
    )
  }
  return (
    <span className={`avatar-frame-custom af-${size}`} aria-hidden="true">
      <img src={frame} alt="" />
    </span>
  )
}
