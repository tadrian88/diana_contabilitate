import * as Dialog from '@radix-ui/react-dialog'
import { zodResolver } from '@hookform/resolvers/zod'
import { Check, Edit3, Scale, X } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import { CLASSIFICATION_DIMENSION_LABELS } from '../../domain/invoice'
import type { ClassificationReviewItem, Invoice } from '../../domain/invoice'
import { useReviewClassification } from './invoice-mutations'

const correctionSchema = z.object({ value: z.string().trim().min(2, 'Completează valoarea corectată.') })
type CorrectionForm = z.infer<typeof correctionSchema>

export function ClassificationTask({ invoice }: { invoice: Invoice }) {
  const task = invoice.task
  const review = useReviewClassification(invoice.id)
  const [correctionItem, setCorrectionItem] = useState<ClassificationReviewItem | null>(null)
  const items = task?.type === 'CLASSIFICATION' ? task.classificationItems ?? [] : []
  const pending = items.filter((item) => item.status === 'PENDING')

  if (!task || task.type !== 'CLASSIFICATION') {
    return <div className="card p-8 text-center"><Check className="mx-auto size-7 text-[var(--success)]" /><h3 className="mt-3 font-bold">Clasificări procesate</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Nu există elemente care necesită intervenție.</p></div>
  }

  return (
    <div className="space-y-4">
      <div className="card flex items-center justify-between p-5">
        <div>
          <Badge tone={pending.length ? 'warning' : 'success'}>{pending.length ? 'Decizie necesară' : 'Task rezolvat'}</Badge>
          <h3 className="mt-3 text-lg font-bold">{task.title}</h3>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">Un singur task grupează toate clasificările incerte ale facturii.</p>
        </div>
        <div className="rounded-xl bg-[var(--surface-subtle)] px-4 py-3 text-right"><div className="text-2xl font-bold tabular-nums">{pending.length}</div><div className="text-xs text-[var(--text-muted)]">probleme rămase</div></div>
      </div>

      {items.map((item) => (
        <article key={item.id} className="card overflow-hidden">
          <div className="flex items-center justify-between border-b border-[var(--border)] px-5 py-3">
            <div><div className="text-xs font-semibold text-[var(--text-muted)]">{item.lineLabel}</div><h4 className="mt-1 font-bold">{CLASSIFICATION_DIMENSION_LABELS[item.dimension]}</h4></div>
            <Badge tone={item.status === 'PENDING' ? 'warning' : 'success'}>{item.status === 'PENDING' ? 'De revizuit' : item.status === 'ACCEPTED' ? 'Acceptat' : 'Corectat'}</Badge>
          </div>
          <div className="grid grid-cols-[0.9fr_1.2fr_1.2fr] gap-0 divide-x divide-[var(--border)]">
            <div className="p-5"><div className="eyebrow">Propunere</div><div className="mt-3 font-semibold">{item.resolvedValue ?? item.proposedValue}</div><Badge tone="info" className="mt-3">Încredere: {item.confidence}</Badge></div>
            <div className="p-5"><div className="eyebrow">Explicație</div><p className="mt-3 text-sm leading-6 text-[var(--text-secondary)]">{item.explanation}</p></div>
            <div className="p-5"><div className="eyebrow">Bază legală</div><div className="mt-3 flex gap-2 rounded-lg border border-[var(--warning-border)] bg-[var(--warning-soft)] p-3 text-xs leading-5 text-[var(--warning)]"><Scale className="mt-0.5 size-4 shrink-0" />{item.legalBasis}</div></div>
          </div>
          {item.status === 'PENDING' && (
            <div className="flex gap-3 border-t border-[var(--border)] bg-[var(--surface-subtle)] px-5 py-4">
              <Button onClick={() => review.mutate({ itemId: item.id })} disabled={review.isPending}><Check className="size-4" />Acceptă propunerea</Button>
              <Button variant="secondary" onClick={() => setCorrectionItem(item)}><Edit3 className="size-4" />Corectează</Button>
            </div>
          )}
        </article>
      ))}

      <CorrectionDialog item={correctionItem} onOpenChange={(open) => !open && setCorrectionItem(null)} onSubmit={(value) => {
        if (correctionItem) review.mutate({ itemId: correctionItem.id, value })
        setCorrectionItem(null)
      }} />
    </div>
  )
}

function CorrectionDialog({ item, onOpenChange, onSubmit }: { item: ClassificationReviewItem | null; onOpenChange: (open: boolean) => void; onSubmit: (value: string) => void }) {
  const { register, handleSubmit, formState: { errors }, reset } = useForm<CorrectionForm>({ resolver: zodResolver(correctionSchema), defaultValues: { value: '' } })

  return (
    <Dialog.Root open={Boolean(item)} onOpenChange={(open) => { onOpenChange(open); if (!open) reset() }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40" />
        <Dialog.Content className="dialog-content fixed left-1/2 top-1/2 z-50 w-[520px] -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl">
          <Dialog.Title className="text-lg font-bold">Corectează clasificarea</Dialog.Title>
          <Dialog.Description className="mt-1 text-sm text-[var(--text-secondary)]">Corecția se aplică numai revizuirii facturii și nu modifică regulile.</Dialog.Description>
          <form className="mt-5" onSubmit={handleSubmit(({ value }) => onSubmit(value))}>
            <label htmlFor="correction-value" className="text-sm font-semibold">Valoare corectată</label>
            <input id="correction-value" autoFocus className="mt-2 h-11 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]" {...register('value')} />
            {errors.value && <p className="mt-2 text-xs text-[var(--danger)]" role="alert">{errors.value.message}</p>}
            <div className="mt-6 flex justify-end gap-3"><Dialog.Close asChild><Button type="button" variant="ghost">Anulează</Button></Dialog.Close><Button type="submit">Salvează corecția</Button></div>
          </form>
          <Dialog.Close className="absolute right-4 top-4 rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-subtle)]" aria-label="Închide"><X className="size-4" /></Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
