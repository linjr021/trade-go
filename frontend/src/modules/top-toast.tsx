// @ts-nocheck

export function TopToast({ toast }) {
  if (!toast?.visible) return null
  return (
    <div className={`top-toast ${toast.type}`} role="status" aria-live="polite">
      {toast.message}
    </div>
  )
}
