import { useEffect } from 'react'

export function useCloseOnOutside(
  open: boolean,
  ref: { current: HTMLElement | null } | null | undefined,
  onClose?: () => void,
) {
  useEffect(() => {
    if (!open) return undefined
    const onDocMouseDown = (e: MouseEvent) => {
      const targetNode = e.target as Node | null
      if (ref?.current && targetNode && !ref.current.contains(targetNode)) {
        onClose?.()
      }
    }
    const onDocKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose?.()
    }
    document.addEventListener('mousedown', onDocMouseDown)
    document.addEventListener('keydown', onDocKeyDown)
    return () => {
      document.removeEventListener('mousedown', onDocMouseDown)
      document.removeEventListener('keydown', onDocKeyDown)
    }
  }, [open, ref, onClose])
}
