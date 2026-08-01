// 品牌标记：蓝色圆角方块 + 白色双闪光（spark）字形，呼应 AI / agent / 解码主题。
// 背景取 --blue，随主题皮肤（蓝 / 紫 / 薄荷）自动切换；在浅色与深色页头上均清晰。
// 纯手绘 SVG，无外部 / 生成位图，可无损缩放；size 控制尺寸，title 提供可访问名称。

export function BrandMark({
  size = 32,
  className,
  title,
}: {
  size?: number
  className?: string
  title?: string
}) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 32 32"
      role={title ? 'img' : undefined}
      aria-hidden={title ? undefined : true}
      aria-label={title}
    >
      <rect width="32" height="32" rx="8" style={{ fill: 'var(--blue)' }} />
      <path
        fill="#fff"
        d="M14.5 8 Q15.94 15.56 23.5 17 Q15.94 18.44 14.5 26 Q13.06 18.44 5.5 17 Q13.06 15.56 14.5 8 Z"
      />
      <path
        fill="#fff"
        fillOpacity="0.9"
        d="M24.5 3.5 Q25.22 7.28 29 8 Q25.22 8.72 24.5 12.5 Q23.78 8.72 20 8 Q23.78 7.28 24.5 3.5 Z"
      />
    </svg>
  )
}
