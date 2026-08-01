// 完整的多主题（皮肤）系统。
//
// 这里是所有主题的唯一数据源：每个主题都定义了明/暗两套完整的 CSS 变量
// （包括强调色、表面、边框、文字层级）以及一张会铺满整个 App 背景的
// `--app-bg`（低对比度的 CSS 渐变，保证内容依旧清晰可读）。
//
// 应用方式：`applyTheme` 会把选中主题、对应明/暗模式的变量以内联自定义属性的
// 形式写到 <html> 上（内联优先级高于样式表，因此可以覆盖 index.css 里的默认值）。
// UI（ThemePanel）则直接遍历 `themes` 渲染色板 + 名称 + 描述。

export type ThemeId = 'ocean' | 'violet' | 'mint' | 'graphite' | 'sunset'

export type ThemeVars = Record<string, string>

export interface Theme {
  id: ThemeId
  /** 中文名称，展示在切换器里。 */
  label: string
  /** 一句话描述主题气质。 */
  description: string
  /** 切换器里的预览色板（CSS 渐变），直观体现主题的背景/强调色。 */
  swatch: string
  /** 浅色模式下的全部 CSS 变量（含 --app-bg）。 */
  light: ThemeVars
  /** 深色模式下的全部 CSS 变量（含 --app-bg）。 */
  dark: ThemeVars
}

// 明色模式的中性文字色（各主题共用，保证阅读一致性）。
const inkLight = {
  '--ink': '#1d1d1f',
  '--muted': '#6e6e73',
  '--text-secondary': '#5f5f65',
  '--text-tertiary': '#8b8b90',
} satisfies ThemeVars

// 深色模式的中性文字色。
const inkDark = {
  '--ink': '#f5f5f7',
  '--muted': '#b6b6bd',
  '--text-secondary': '#c5c5ca',
  '--text-tertiary': '#9999a1',
} satisfies ThemeVars

export const themes: Theme[] = [
  {
    id: 'ocean',
    label: '海蓝',
    description: '沉静的经典蓝，默认主题',
    swatch: 'linear-gradient(135deg, #0066cc 0%, #4aa3ff 100%)',
    light: {
      ...inkLight,
      '--blue': '#0066cc',
      '--blue-hover': '#0071e3',
      '--line': '#e4e8ef',
      '--canvas': '#f4f6fa',
      '--pearl': '#fafbfd',
      '--page': '#ffffff',
      '--surface': '#ffffff',
      '--surface-muted': '#f6f9fc',
      '--soft': '#eef3f9',
      '--soft-blue': '#edf6fd',
      '--focus-line': '#a9ccee',
      '--app-bg':
        'radial-gradient(1200px 640px at 8% -12%, rgba(0,102,204,.10), transparent 60%), radial-gradient(1000px 560px at 108% 0%, rgba(56,142,232,.07), transparent 58%), linear-gradient(180deg, #ffffff 0%, #f5f9fd 100%)',
    },
    dark: {
      ...inkDark,
      '--blue': '#2f88e8',
      '--blue-hover': '#4a9bf0',
      '--line': '#2b323c',
      '--canvas': '#12151a',
      '--pearl': '#171b21',
      '--page': '#0f1216',
      '--surface': '#171b21',
      '--surface-muted': '#1d222a',
      '--soft': '#232a34',
      '--soft-blue': '#17293c',
      '--focus-line': '#3d6a9c',
      '--app-bg':
        'radial-gradient(1200px 640px at 8% -12%, rgba(56,142,232,.16), transparent 60%), radial-gradient(1000px 560px at 108% 0%, rgba(20,70,140,.22), transparent 58%), linear-gradient(180deg, #0f1216 0%, #0b0e12 100%)',
    },
  },
  {
    id: 'violet',
    label: '暮紫',
    description: '富有创造力的柔和紫',
    swatch: 'linear-gradient(135deg, #7259c8 0%, #b89ff5 100%)',
    light: {
      ...inkLight,
      '--blue': '#7259c8',
      '--blue-hover': '#8068d9',
      '--line': '#eae7f3',
      '--canvas': '#f6f4fb',
      '--pearl': '#fbfafe',
      '--page': '#ffffff',
      '--surface': '#ffffff',
      '--surface-muted': '#f8f6fc',
      '--soft': '#f1edfa',
      '--soft-blue': '#f0edff',
      '--focus-line': '#c1b6f2',
      '--app-bg':
        'radial-gradient(1200px 640px at 6% -12%, rgba(114,89,200,.12), transparent 60%), radial-gradient(1000px 560px at 108% 0%, rgba(160,110,220,.08), transparent 58%), linear-gradient(180deg, #ffffff 0%, #f7f4fc 100%)',
    },
    dark: {
      ...inkDark,
      '--blue': '#9d82ea',
      '--blue-hover': '#ac93f0',
      '--line': '#322c40',
      '--canvas': '#141019',
      '--pearl': '#1a1522',
      '--page': '#110d17',
      '--surface': '#1a1522',
      '--surface-muted': '#201a2a',
      '--soft': '#271f34',
      '--soft-blue': '#241b38',
      '--focus-line': '#6b58a0',
      '--app-bg':
        'radial-gradient(1200px 640px at 6% -12%, rgba(150,120,230,.18), transparent 60%), radial-gradient(1000px 560px at 108% 0%, rgba(90,55,160,.22), transparent 58%), linear-gradient(180deg, #110d17 0%, #0d0a12 100%)',
    },
  },
  {
    id: 'mint',
    label: '薄荷',
    description: '清新通透的自然绿',
    swatch: 'linear-gradient(135deg, #16846a 0%, #52cca9 100%)',
    light: {
      ...inkLight,
      '--blue': '#16846a',
      '--blue-hover': '#1b987b',
      '--line': '#dfece7',
      '--canvas': '#f2f8f5',
      '--pearl': '#f9fcfb',
      '--page': '#ffffff',
      '--surface': '#ffffff',
      '--surface-muted': '#f5faf8',
      '--soft': '#e9f4ef',
      '--soft-blue': '#e8f7f1',
      '--focus-line': '#9fd8c5',
      '--app-bg':
        'radial-gradient(1200px 640px at 8% -12%, rgba(22,132,106,.10), transparent 60%), radial-gradient(1000px 560px at 108% 0%, rgba(64,191,156,.08), transparent 58%), linear-gradient(180deg, #ffffff 0%, #f3f9f6 100%)',
    },
    dark: {
      ...inkDark,
      '--blue': '#2eb894',
      '--blue-hover': '#3fc7a3',
      '--line': '#26332e',
      '--canvas': '#0f1512',
      '--pearl': '#141b17',
      '--page': '#0c110e',
      '--surface': '#141b17',
      '--surface-muted': '#19211c',
      '--soft': '#1f2a25',
      '--soft-blue': '#16271f',
      '--focus-line': '#2f7862',
      '--app-bg':
        'radial-gradient(1200px 640px at 8% -12%, rgba(64,191,156,.16), transparent 60%), radial-gradient(1000px 560px at 108% 0%, rgba(18,110,86,.22), transparent 58%), linear-gradient(180deg, #0c110e 0%, #090d0b 100%)',
    },
  },
  {
    id: 'graphite',
    label: '石墨',
    description: '克制的中性灰，专业冷静',
    swatch: 'linear-gradient(135deg, #3f3f46 0%, #9a9aa4 100%)',
    light: {
      ...inkLight,
      '--blue': '#3f3f46',
      '--blue-hover': '#52525b',
      '--line': '#e5e5e8',
      '--canvas': '#f4f4f5',
      '--pearl': '#fafafa',
      '--page': '#ffffff',
      '--surface': '#ffffff',
      '--surface-muted': '#f7f7f8',
      '--soft': '#eeeef0',
      '--soft-blue': '#ededf0',
      '--focus-line': '#b8b8be',
      '--app-bg':
        'radial-gradient(1200px 640px at 50% -14%, rgba(0,0,0,.055), transparent 62%), linear-gradient(180deg, #ffffff 0%, #f3f3f4 100%)',
    },
    dark: {
      ...inkDark,
      '--blue': '#7f7f8a',
      '--blue-hover': '#93939d',
      '--line': '#2d2d31',
      '--canvas': '#131315',
      '--pearl': '#18181b',
      '--page': '#101012',
      '--surface': '#18181b',
      '--surface-muted': '#1e1e21',
      '--soft': '#26262a',
      '--soft-blue': '#232327',
      '--focus-line': '#5a5a62',
      '--app-bg':
        'radial-gradient(1200px 640px at 50% -14%, rgba(255,255,255,.05), transparent 62%), linear-gradient(180deg, #101012 0%, #0c0c0e 100%)',
    },
  },
  {
    id: 'sunset',
    label: '暖阳',
    description: '温暖明亮的珊瑚琥珀',
    swatch: 'linear-gradient(135deg, #d1502a 0%, #f5a15a 100%)',
    light: {
      ...inkLight,
      '--blue': '#cf4f28',
      '--blue-hover': '#e05e35',
      '--line': '#f0e5df',
      '--canvas': '#fbf5f1',
      '--pearl': '#fefbf9',
      '--page': '#ffffff',
      '--surface': '#ffffff',
      '--surface-muted': '#fcf7f4',
      '--soft': '#f7ece5',
      '--soft-blue': '#fdefe6',
      '--focus-line': '#f0b79c',
      '--app-bg':
        'radial-gradient(1200px 640px at 8% -12%, rgba(207,79,40,.10), transparent 60%), radial-gradient(1000px 560px at 108% 0%, rgba(245,161,90,.10), transparent 58%), linear-gradient(180deg, #ffffff 0%, #fdf5ef 100%)',
    },
    dark: {
      ...inkDark,
      '--blue': '#e8703f',
      '--blue-hover': '#f0824f',
      '--line': '#38302c',
      '--canvas': '#181310',
      '--pearl': '#1e1815',
      '--page': '#141010',
      '--surface': '#1e1815',
      '--surface-muted': '#241d19',
      '--soft': '#2d2420',
      '--soft-blue': '#331f16',
      '--focus-line': '#8a5136',
      '--app-bg':
        'radial-gradient(1200px 640px at 8% -12%, rgba(232,112,63,.16), transparent 60%), radial-gradient(1000px 560px at 108% 0%, rgba(150,70,30,.22), transparent 58%), linear-gradient(180deg, #141010 0%, #0f0c0b 100%)',
    },
  },
]

const themeMap: Record<ThemeId, Theme> = themes.reduce(
  (acc, theme) => {
    acc[theme.id] = theme
    return acc
  },
  {} as Record<ThemeId, Theme>,
)

/** 判断任意值是否是合法的主题 id（用于校验 localStorage）。 */
export function isThemeId(value: unknown): value is ThemeId {
  return typeof value === 'string' && value in themeMap
}

/** 取主题，未知 id 回退到默认海蓝。 */
export function getTheme(id: ThemeId): Theme {
  return themeMap[id] ?? themes[0]
}

/**
 * 把某个主题在指定明/暗模式下的全部变量写到目标元素（通常是 <html>）。
 * 以内联自定义属性形式设置，优先级高于样式表默认值。
 */
export function applyTheme(root: HTMLElement, id: ThemeId, dark: boolean): void {
  const vars = dark ? getTheme(id).dark : getTheme(id).light
  for (const [name, value] of Object.entries(vars)) {
    root.style.setProperty(name, value)
  }
}
