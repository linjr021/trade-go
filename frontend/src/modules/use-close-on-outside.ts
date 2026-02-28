// @ts-nocheck
import { useEffect } from 'react'

export function useCloseOnOutside(open, ref, onClose) {
  useEffect(() => {
    if (!open) return undefined
    const onDocMouseDown = (e) => {
      if (ref?.current && !ref.current.contains(e.target)) {
        onClose?.()
      }
    }
    const onDocKeyDown = (e) => {
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
