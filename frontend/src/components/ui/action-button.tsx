import { Loader2 } from 'lucide-react'
import { Button as ShadButton } from '@/components/ui/button'

export function ActionButton({ loading = false, disabled, children, ...props }) {
  return (
    <ShadButton disabled={Boolean(disabled) || loading} {...props}>
      {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
      {children}
    </ShadButton>
  )
}
