// 头像框数据与判定（纯数据模块，无组件导出）。
//
// avatar_frame 存储值：'' | 预设 id | 'custom:<objectKey>'。

export const AVATAR_FRAME_IDS = [
  'zodiac-aries',
  'zodiac-taurus',
  'zodiac-gemini',
  'zodiac-cancer',
  'zodiac-leo',
  'zodiac-virgo',
  'zodiac-libra',
  'zodiac-scorpio',
  'zodiac-sagittarius',
  'zodiac-capricorn',
  'zodiac-aquarius',
  'zodiac-pisces',
] as const

export type AvatarFrameId = (typeof AVATAR_FRAME_IDS)[number]

export type AvatarFramePreset = {
  id: AvatarFrameId
  label: string // 中文星座名
  glyph: string // 星座符号
}

// 12 星座预设。label/glyph 用于选择器与 SVG 内符号。
export const AVATAR_FRAMES: AvatarFramePreset[] = [
  { id: 'zodiac-aries', label: '白羊座', glyph: '♈' },
  { id: 'zodiac-taurus', label: '金牛座', glyph: '♉' },
  { id: 'zodiac-gemini', label: '双子座', glyph: '♊' },
  { id: 'zodiac-cancer', label: '巨蟹座', glyph: '♋' },
  { id: 'zodiac-leo', label: '狮子座', glyph: '♌' },
  { id: 'zodiac-virgo', label: '处女座', glyph: '♍' },
  { id: 'zodiac-libra', label: '天秤座', glyph: '♎' },
  { id: 'zodiac-scorpio', label: '天蝎座', glyph: '♏' },
  { id: 'zodiac-sagittarius', label: '射手座', glyph: '♐' },
  { id: 'zodiac-capricorn', label: '摩羯座', glyph: '♑' },
  { id: 'zodiac-aquarius', label: '水瓶座', glyph: '♒' },
  { id: 'zodiac-pisces', label: '双鱼座', glyph: '♓' },
]

const PRESET_IDS = new Set<string>(AVATAR_FRAME_IDS)

export function isPresetFrameId(value: string): value is AvatarFrameId {
  return PRESET_IDS.has(value)
}

export function findAvatarFrame(id: string): AvatarFramePreset | undefined {
  return AVATAR_FRAMES.find((frame) => frame.id === id)
}
