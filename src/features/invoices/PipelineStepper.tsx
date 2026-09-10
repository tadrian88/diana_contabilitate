import { AlertTriangle, Check, Circle, Copy, LoaderCircle, Pause } from 'lucide-react'
import type { Invoice, PipelineStatus } from '../../domain/invoice'
import { PIPELINE_LABELS, TERMINAL_STATES } from '../../domain/invoice'
import { cn } from '../../lib/cn'

interface PipelineStepperProps {
  invoice: Pick<Invoice, 'pipelineStatus' | 'pipelinePath' | 'sagaStatus'>
}

export function PipelineStepper({ invoice }: PipelineStepperProps) {
  const currentIndex = invoice.pipelinePath.indexOf(invoice.pipelineStatus)
  const failedExport = invoice.sagaStatus === 'FAILED'

  return (
    <section className="card p-5" aria-labelledby="pipeline-title">
      <div className="flex items-start justify-between gap-6">
        <div>
          <p className="eyebrow">Pipeline factură</p>
          <h2 id="pipeline-title" className="mt-1 text-base font-bold">{PIPELINE_LABELS[invoice.pipelineStatus]}</h2>
        </div>
        <div className="text-right text-xs text-[var(--text-muted)]">
          {TERMINAL_STATES.includes(invoice.pipelineStatus) ? 'Stare terminală' : failedExport ? 'Export simulat eșuat' : `${currentIndex + 1} din ${invoice.pipelinePath.length} etape`}
        </div>
      </div>

      <ol className="mt-6 flex" aria-label="Etapele procesării facturii">
        {invoice.pipelinePath.map((status, index) => {
          const completed = index < currentIndex || (index === currentIndex && TERMINAL_STATES.includes(status))
          const current = index === currentIndex && !completed
          const waiting = current && (status === 'AWAITING_CONTRACT' || status === 'AWAITING_MATCH_CONFIRM' || status === 'AWAITING_REVIEW')
          const duplicate = status === 'DUPLICATE'
          const exporting = current && status === 'EXPORTING'
          const state = completed ? 'completed' : waiting ? 'blocked' : current ? 'current' : 'pending'
          const Icon = duplicate ? Copy : completed ? Check : waiting ? Pause : exporting ? LoaderCircle : current ? Circle : Circle
          return (
            <li key={`${status}-${index}`} className="relative flex min-w-0 flex-1 flex-col items-center text-center" data-state={state}>
              {index > 0 && <span aria-hidden="true" className={cn('absolute left-0 top-3.5 h-0.5 w-1/2', index <= currentIndex ? 'bg-[var(--success)]' : 'bg-[var(--border)]')} />}
              {index < invoice.pipelinePath.length - 1 && <span aria-hidden="true" className={cn('absolute right-0 top-3.5 h-0.5 w-1/2', index < currentIndex ? 'bg-[var(--success)]' : 'bg-[var(--border)]')} />}
              <span className={cn(
                'relative z-10 grid size-7 place-items-center rounded-full border-2 bg-[var(--surface)]',
                completed && !duplicate && 'border-[var(--success)] bg-[var(--success)] text-white',
                duplicate && 'border-[var(--danger)] bg-[var(--danger)] text-white',
                waiting && 'border-[var(--warning)] bg-[var(--warning-soft)] text-[var(--warning)]',
                current && !waiting && !duplicate && 'border-[var(--accent)] text-[var(--accent)]',
                !completed && !current && 'border-[var(--border-strong)] text-[var(--text-muted)]',
              )}>
                <Icon className={cn('size-3.5', exporting && 'animate-spin')} aria-hidden="true" />
              </span>
              <span className={cn('mt-2 max-w-[90px] text-[10px] font-semibold leading-3', (current || completed) ? 'text-[var(--text-secondary)]' : 'text-[var(--text-muted)]')}>{PIPELINE_LABELS[status]}</span>
              <span className="sr-only">{completed ? 'finalizată' : waiting ? 'blocată' : current ? 'curentă' : 'în așteptare'}</span>
            </li>
          )
        })}
      </ol>

      {(invoice.pipelineStatus === 'AWAITING_CONTRACT' || invoice.pipelineStatus === 'AWAITING_MATCH_CONFIRM' || invoice.pipelineStatus === 'AWAITING_REVIEW') && (
        <div className="mt-5 flex items-start gap-3 rounded-xl border border-[var(--warning-border)] bg-[var(--warning-soft)] p-3 text-sm text-[var(--warning)]" role="status">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" />
          <div><strong>Procesare oprită.</strong> {invoice.pipelineStatus === 'AWAITING_CONTRACT' ? 'Este necesar un contract extern.' : invoice.pipelineStatus === 'AWAITING_MATCH_CONFIRM' ? 'Contabilul trebuie să confirme contractul.' : 'Clasificările incerte trebuie revizuite.'}</div>
        </div>
      )}
      {invoice.pipelineStatus === 'DUPLICATE' && (
        <div className="mt-5 flex items-start gap-3 rounded-xl border border-[var(--danger-border)] bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]" role="status">
          <Copy className="mt-0.5 size-4 shrink-0" />
          <div><strong>Procesare încheiată.</strong> Factura a fost identificată drept duplicat și nu continuă către SAGA.</div>
        </div>
      )}
    </section>
  )
}
