import { useEffect, useRef } from 'react'
import data from '@emoji-mart/data'
import i18n from '@emoji-mart/data/i18n/zh.json'
import { Picker } from 'emoji-mart'

type EmojiSelection = {
  native: string
}

// 这里不使用 emoji-mart 的 onClickOutside：它在任意文档点击时都会触发，
// 与外层开关按钮的 toggle 互相干扰。外部点击关闭由调用方统一处理。
export default function EmojiMartPicker({
  onSelect,
}: {
  onSelect: (emoji: string) => void
}) {
  const rootRef = useRef<HTMLSpanElement | null>(null)
  const onSelectRef = useRef(onSelect)
  onSelectRef.current = onSelect

  useEffect(() => {
    const picker = new Picker({
      data,
      i18n,
      locale: 'zh',
      set: 'native',
      theme: 'auto',
      autoFocus: true,
      dynamicWidth: true,
      maxFrequentRows: 2,
      previewPosition: 'none',
      skinTonePosition: 'search',
      onEmojiSelect: (emoji: EmojiSelection) => onSelectRef.current(emoji.native),
    }) as unknown as HTMLElement
    rootRef.current?.appendChild(picker)
    return () => picker.remove()
  }, [])

  return <span className="emoji-mart-host" ref={rootRef} />
}
