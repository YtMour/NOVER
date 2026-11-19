import { useCallback, useEffect, useRef, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'
import { ChevronLeft, ChevronRight, X } from 'lucide-react'

export type LightboxImage = {
  id: string
  src: string
  alt: string
}

type LightboxProps = {
  images: LightboxImage[]
  index: number
  label: string
  onIndexChange: (index: number) => void
  onClose: () => void
  /** 关闭后需要归还焦点的触发元素（画廊按钮）。 */
  returnFocus?: HTMLElement | null
}

/**
 * 商品效果图光箱：Esc/遮罩关闭，左右方向键切换，焦点陷阱与 aria-modal，
 * 打开时锁定 body 滚动，关闭后把焦点还给触发按钮；动效由 CSS 统一降级。
 */
export function Lightbox({ images, index, label, onIndexChange, onClose, returnFocus }: LightboxProps) {
  const dialogRef = useRef<HTMLDivElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const hasMultiple = images.length > 1
  const image = images[index]

  const go = useCallback((delta: number) => {
    const total = images.length
    onIndexChange((index + delta + total) % total)
  }, [images.length, index, onIndexChange])

  useEffect(() => {
    const trigger = returnFocus ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null)
    const body = document.body
    const previousOverflow = body.style.overflow
    const previousPaddingRight = body.style.paddingRight
    const scrollbarWidth = window.innerWidth - document.documentElement.clientWidth
    body.style.overflow = 'hidden'
    if (scrollbarWidth > 0) body.style.paddingRight = `${scrollbarWidth}px`
    closeRef.current?.focus({ preventScroll: true })
    return () => {
      body.style.overflow = previousOverflow
      body.style.paddingRight = previousPaddingRight
      trigger?.focus({ preventScroll: true })
    }
  }, [returnFocus])

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (event.key === 'ArrowRight' && hasMultiple) {
      event.preventDefault()
      go(1)
      return
    }
    if (event.key === 'ArrowLeft' && hasMultiple) {
      event.preventDefault()
      go(-1)
      return
    }
    if (event.key !== 'Tab') return
    // 焦点陷阱：只在光箱自身的可交互元素之间循环。
    const focusable = dialogRef.current?.querySelectorAll<HTMLElement>('button:not([disabled])')
    if (!focusable?.length) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  if (!image) return null

  // 挂到 body：页面入场动画会留下 transform，不能让 position: fixed 相对它定位。
  return createPortal(
    <div
      className="lightbox"
      role="presentation"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div
        ref={dialogRef}
        className="lightbox-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={label}
        onClick={(event) => {
          if (event.target === event.currentTarget) onClose()
        }}
        onKeyDown={handleKeyDown}
      >
        <button ref={closeRef} className="lightbox-close" type="button" aria-label="关闭图片预览" title="关闭预览（Esc）" onClick={onClose}>
          <X size={19} aria-hidden="true" />
        </button>
        {hasMultiple && (
          <button className="lightbox-nav lightbox-nav--prev" type="button" aria-label="上一张图片" title="上一张（←）" onClick={() => go(-1)}>
            <ChevronLeft size={22} aria-hidden="true" />
          </button>
        )}
        <figure className="lightbox-figure">
          <img src={image.src} alt={image.alt} />
          <figcaption className="lightbox-caption" aria-live="polite">
            <span>{image.alt}</span>
            {hasMultiple && <span className="lightbox-counter">{index + 1} / {images.length}</span>}
          </figcaption>
        </figure>
        {hasMultiple && (
          <button className="lightbox-nav lightbox-nav--next" type="button" aria-label="下一张图片" title="下一张（→）" onClick={() => go(1)}>
            <ChevronRight size={22} aria-hidden="true" />
          </button>
        )}
      </div>
    </div>,
    document.body,
  )
}
