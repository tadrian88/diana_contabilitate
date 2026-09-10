import { cva, type VariantProps } from 'class-variance-authority'
import type { HTMLAttributes } from 'react'
import { cn } from '../../lib/cn'

const badgeVariants = cva('inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold', {
  variants: {
    tone: {
      neutral: 'border-[var(--border)] bg-[var(--surface-subtle)] text-[var(--text-secondary)]',
      info: 'border-[var(--info-border)] bg-[var(--info-soft)] text-[var(--info)]',
      warning: 'border-[var(--warning-border)] bg-[var(--warning-soft)] text-[var(--warning)]',
      success: 'border-[var(--success-border)] bg-[var(--success-soft)] text-[var(--success)]',
      danger: 'border-[var(--danger-border)] bg-[var(--danger-soft)] text-[var(--danger)]',
    },
  },
  defaultVariants: { tone: 'neutral' },
})

export function Badge({ tone, className, ...props }: HTMLAttributes<HTMLSpanElement> & VariantProps<typeof badgeVariants>) {
  return <span className={cn(badgeVariants({ tone }), className)} {...props} />
}

