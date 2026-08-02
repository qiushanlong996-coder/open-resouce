// 头像框渲染层（仅组件导出，数据在 avatarFrameData.ts）。
//
// AvatarFrameLayer 契约：frame 为预设 id 时渲染星座 SVG/装饰；为非空图片 URL 时
// 渲染自定义图片；为空返回 null（调用方回退到等级框）。集成方负责把
// 'custom:<key>' 解析成图片 URL 后再传入。

import { findAvatarFrame } from './avatarFrameData'
import './avatar-frames.css'

export function AvatarFrameLayer({ frame, size = 'md' }: { frame: string; size?: 'sm' | 'md' | 'lg' }) {
  if (!frame) return null
  const preset = findAvatarFrame(frame)
  if (preset) {
    return (
      <span className={`avatar-frame-preset af-botanical af-${size}`} aria-hidden="true">
        <img src="/avatar-frame-botanical.gif" alt="" />
      </span>
    )
  }
  return (
    <span className={`avatar-frame-custom af-${size}`} aria-hidden="true">
      <img src={frame} alt="" />
    </span>
  )
}
