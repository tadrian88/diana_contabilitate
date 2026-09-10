import { CheckCircle2, CircleHelp, Clock3 } from 'lucide-react'
import { Badge } from '../../components/ui/badge'
import type { ClassificationReviewItem, Invoice, LineClassification } from '../../domain/invoice'
import { CLASSIFICATION_DIMENSION_LABELS } from '../../domain/invoice'
import { Link } from 'react-router-dom'
import { ClassificationTask } from './ClassificationTask'

const dimensions: LineClassification['dimension'][] = ['ACCOUNT', 'VAT', 'DEDUCTIBILITY']

export function ClassificationWorkspace({ invoice }: { invoice: Invoice }) {
  if (!hasReachedClassification(invoice)) {
    return <div className="card p-10 text-center"><Clock3 className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Clasificarea nu este încă disponibilă</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Factura trebuie să ajungă la etapa de clasificare înainte ca aceste date să fie afișate.</p></div>
  }

  const uncertainItems = invoice.task?.type === 'CLASSIFICATION' ? invoice.task.classificationItems ?? [] : []
  return (
    <div className="space-y-5">
      {invoice.task?.type === 'CLASSIFICATION' && <ClassificationTask invoice={invoice} />}
      <section className="card overflow-hidden">
        <div className="border-b border-[var(--border)] px-5 py-4"><h3 className="font-bold">Clasificări pe linii</h3><p className="mt-1 text-xs text-[var(--text-muted)]">Dimensiunile sigure sunt vizibile, dar nu necesită acțiune.</p></div>
        <div className="divide-y divide-[var(--border)]">
          {invoice.lines.map((line) => (
            <article key={line.id} className="p-5">
              <div className="flex items-center gap-3"><span className="grid size-7 place-items-center rounded-lg bg-[var(--surface-subtle)] text-xs font-bold">{line.position}</span><div><h4 className="text-sm font-bold">{line.description}</h4><div className="mt-0.5 text-xs text-[var(--text-muted)]">{line.quantity} {line.unit} · {formatMoney(line.netValue.amount, line.netValue.currency)}</div></div></div>
              <div className="mt-4 grid grid-cols-3 gap-3">
                {dimensions.map((dimension) => {
                  const confident = line.classifications.find((item) => item.dimension === dimension)
                  const uncertain = uncertainItems.find((item) => item.lineId === line.id && item.dimension === dimension)
                  return <ClassificationCell key={dimension} label={CLASSIFICATION_DIMENSION_LABELS[dimension]} confident={confident} uncertain={uncertain} invoiceId={invoice.id} />
                })}
              </div>
            </article>
          ))}
        </div>
      </section>
    </div>
  )
}

function ClassificationCell({ label, confident, uncertain, invoiceId }: { label: string; confident?: LineClassification; uncertain?: ClassificationReviewItem; invoiceId: string }) {
  const pending = uncertain?.status === 'PENDING'
  const value = uncertain?.resolvedValue ?? uncertain?.proposedValue ?? confident?.value
  const confidence = uncertain?.confidence ?? confident?.confidence
  const explanation = uncertain?.explanation ?? confident?.explanation
  const legalBasis = uncertain?.legalBasis ?? confident?.legalBasis
  const rule = uncertain?.rule ?? confident?.rule
  return (
    <div className={`rounded-xl border p-4 ${pending ? 'border-[var(--warning-border)] bg-[var(--warning-soft)]' : 'border-[var(--border)] bg-[var(--surface-raised)]'}`}>
      <div className="flex items-start justify-between gap-2"><span className="eyebrow">{label}</span>{pending ? <CircleHelp className="size-4 text-[var(--warning)]" /> : <CheckCircle2 className="size-4 text-[var(--success)]" />}</div>
      <div className="mt-3 text-sm font-bold">{value ?? 'Informație indisponibilă'}</div>
      {confidence && <div className="mt-1 text-[11px] text-[var(--text-secondary)]">Încredere: {confidence}</div>}
      {explanation && <p className="mt-3 text-xs leading-5 text-[var(--text-secondary)]">{explanation}</p>}
      {legalBasis && <p className="mt-3 border-t border-[var(--border)] pt-3 text-[10px] leading-4 text-[var(--text-muted)]">{legalBasis}</p>}
      {rule && <Link to={`/rules/${rule.ruleId}?version=${rule.version}&returnTo=${encodeURIComponent(`/invoices/${invoiceId}?tab=classification`)}`} className="mt-3 block rounded-lg border border-[var(--border)] bg-[var(--surface)] p-2 text-[10px] font-semibold text-[var(--accent)] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Origine regulă: {rule.origin === 'GLOBAL' ? 'Regulă globală' : 'Override client'} · {rule.reference} · v{rule.version}</Link>}
      <Badge className="mt-3" tone={pending ? 'warning' : 'success'}>{pending ? 'De revizuit' : uncertain?.status === 'CORRECTED' ? 'Corectat' : 'Acceptat automat'}</Badge>
    </div>
  )
}

function hasReachedClassification(invoice: Invoice) {
  const classificationIndex = invoice.pipelinePath.indexOf('CLASSIFIED')
  const currentIndex = invoice.pipelinePath.indexOf(invoice.pipelineStatus)
  return classificationIndex >= 0 && currentIndex >= classificationIndex
}

function formatMoney(amount: number, currency: string) { return new Intl.NumberFormat('ro-RO', { style: 'currency', currency }).format(amount) }
