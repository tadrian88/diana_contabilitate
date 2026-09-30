import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { useState } from 'react'
import { Button } from '../../components/ui/button'
import type { LearnedServiceAlias } from '../../repositories/invoiceRepository'
import { useRevokeCommercialServiceAlias } from './contract-hooks'

// Revoking a learned wording asks first and says what it changes: future
// invoices are no longer mapped by it, while a mapping confirmed on an
// invoice stays with that invoice.
export function RevokeAliasButton({ clientId, alias, appearance }: { clientId: string; alias: LearnedServiceAlias; appearance: 'link' | 'button' }) {
  const [open, setOpen] = useState(false)
  const revoke = useRevokeCommercialServiceAlias(clientId)
  const service = alias.serviceLabel ?? alias.serviceId
  return (
    <Dialog.Root open={open} onOpenChange={(next) => { setOpen(next); if (!next) revoke.reset() }}>
      <Dialog.Trigger asChild>
        {appearance === 'link'
          ? <button type="button" className="font-semibold text-[var(--accent)] underline">Revocă</button>
          : <Button type="button" variant="secondary" size="sm">Revocă</Button>}
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40" />
        <Dialog.Content className="dialog-content fixed left-1/2 top-1/2 z-50 w-[min(500px,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl">
          <Dialog.Title className="text-lg font-bold">Revoci formularea „{alias.normalizedLabel}”?</Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-[var(--text-secondary)]">
            Facturile viitoare din acest contract cu această formulare nu vor mai fi asociate automat cu „{service}”: Diana va cere din nou alegerea serviciului. Asocierile confirmate direct pe o factură rămân valabile pentru acea factură.
          </Dialog.Description>
          {revoke.isError && <p role="alert" className="mt-3 rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">Asocierea nu a putut fi revocată.</p>}
          <div className="mt-6 flex justify-end gap-3">
            <Dialog.Close asChild><Button variant="ghost">Anulează</Button></Dialog.Close>
            <Button disabled={revoke.isPending} onClick={() => revoke.mutate(alias.id, { onSuccess: () => setOpen(false) })}>{revoke.isPending ? 'Se revocă…' : 'Revocă formularea'}</Button>
          </div>
          <Dialog.Close className="absolute right-4 top-4 rounded-md p-1 text-[var(--text-muted)]" aria-label="Închide"><X className="size-4" /></Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
