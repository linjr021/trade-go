import { Loader2 } from 'lucide-react'
import { Button as ShadButton } from '@/components/ui/button'
import type { ReactNode } from 'react'

type ActionButtonProps = {
  loading?: boolean
  disabled?: boolean
  children?: ReactNode
  [key: string]: any
}

export function ActionButton({ loading = false, disabled = false, children, ...props }: ActionButtonProps) {
  return (
    <ShadButton disabled={Boolean(disabled) || loading} {...props}>
      {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
      {children}
    </ShadButton>
  )
}
