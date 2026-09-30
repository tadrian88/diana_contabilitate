import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { useState } from 'react'
import { Button, type ButtonProps } from '../../components/ui/button'
import { useContinueWithoutContract } from './invoice-mutations'

export const CONTRACT_WAIVER_MIN_REASON = 10
export const CONTRACT_WAIVER_MAX_REASON = 500

/**
 * Decizia motivată de a continua o factură fără contract (D-120). Motivul se
 * păstrează în istoric împreună cu autorul; serverul îl validează din nou.
 */
export function ContinueWithoutContractDialog({ invoiceId, size, onContinued }: { invoiceId: string; size?: ButtonProps['size']; onContinued?: () => void }) {
  const continueWithoutContract = useContinueWithoutContract(invoiceId)
  const [open, setOpen] = useState(false)
  const [reason, setReason] = useState('')
  const trimmed = reason.trim()
  const valid = trimmed.length >= CONTRACT_WAIVER_MIN_REASON && trimmed.length <= CONTRACT_WAIVER_MAX_REASON

  const onOpenChange = (next: boolean) => {
    setOpen(next)
    if (!next) {
      setReason('')
      continueWithoutContract.reset()
    }
  }
  const confirm = () => {
    if (!valid) return
    continueWithoutContract.mutate(trimmed, {
      onSuccess: () => {
        onOpenChange(false)
        onContinued?.()
      },
    })
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Trigger asChild><Button variant="secondary" size={size}>Continuă fără contract</Button></Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40" />
        <Dialog.Content className="dialog-content fixed left-1/2 top-1/2 z-50 w-[min(520px,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl">
          <Dialog.Title className="text-lg font-bold">Continui fără contract?</Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-[var(--text-secondary)]">Factura trece mai departe fără contract asociat: nu va fi verificată față de un contract, iar clasificarea contabilă pornește normal. Decizia se salvează în istoric, cu numele tău și motivul. Dacă încarci ulterior un contract pentru acest furnizor, factura nu va mai fi reevaluată automat.</Dialog.Description>
          <label className="mt-5 block text-sm font-semibold">Motivul deciziei
            <textarea value={reason} maxLength={CONTRACT_WAIVER_MAX_REASON} onChange={(event) => setReason(event.target.value)} placeholder="Ex.: achiziție punctuală, furnizorul nu lucrează pe bază de contract" className="mt-2 min-h-24 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] p-3 font-normal outline-none focus:ring-2 focus:ring-[var(--focus)]" />
          </label>
          <p className="mt-1 text-xs text-[var(--text-muted)]">Minimum {CONTRACT_WAIVER_MIN_REASON} caractere.</p>
          {continueWithoutContract.isError && <p role="alert" className="mt-3 rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">Decizia nu a putut fi salvată. Reîncarcă factura și încearcă din nou.</p>}
          <div className="mt-6 flex justify-end gap-3">
            <Dialog.Close asChild><Button variant="ghost">Anulează</Button></Dialog.Close>
            <Button disabled={!valid || continueWithoutContract.isPending} onClick={confirm}>{continueWithoutContract.isPending ? 'Se salvează…' : 'Continuă fără contract'}</Button>
          </div>
          <Dialog.Close className="absolute right-4 top-4 rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-subtle)]" aria-label="Închide"><X className="size-4" /></Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
