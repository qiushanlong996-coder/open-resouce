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
      <rect width="32" height="32" rx="9" style={{ fill: 'color-mix(in srgb, var(--blue) 16%, var(--surface))' }} />
      <path
        d="M13 9.5 7.5 16l5.5 6.5M19 9.5l5.5 6.5-5.5 6.5"
        fill="none"
        stroke="var(--blue)"
        strokeWidth="2.25"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="16" cy="16" r="2" fill="var(--blue)" />
    </svg>
  )
}
