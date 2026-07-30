import { useEffect, useRef } from 'react'
import data from '@emoji-mart/data'
import i18n from '@emoji-mart/data/i18n/zh.json'
import { Picker } from 'emoji-mart'

type EmojiSelection = {
  native: string
}

export default function EmojiMartPicker({
  onSelect,
  onClose,
}: {
  onSelect: (emoji: string) => void
  onClose: () => void
}) {
  const rootRef = useRef<HTMLSpanElement | null>(null)
  const onSelectRef = useRef(onSelect)
  const onCloseRef = useRef(onClose)
  onSelectRef.current = onSelect
  onCloseRef.current = onClose

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
      onClickOutside: () => onCloseRef.current(),
      onEmojiSelect: (emoji: EmojiSelection) => onSelectRef.current(emoji.native),
    }) as unknown as HTMLElement
    rootRef.current?.appendChild(picker)
    return () => picker.remove()
  }, [])

  return <span className="emoji-mart-host" ref={rootRef} />
}
