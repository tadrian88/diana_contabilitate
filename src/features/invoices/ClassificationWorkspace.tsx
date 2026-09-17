import { CheckCircle2, CircleHelp, Clock3 } from 'lucide-react'
import { Badge } from '../../components/ui/badge'
import type { ClassificationReviewItem, Invoice, LineClassification } from '../../domain/invoice'
import { CLASSIFICATION_DIMENSION_LABELS } from '../../domain/invoice'
import { Link } from 'react-router-dom'
import { displayDomainValue } from './domain-decision-view'
import { ClassificationTask } from './ClassificationTask'

const legacyDimensions: LineClassification['dimension'][] = ['ACCOUNT', 'VAT', 'DEDUCTIBILITY']
const domainDimensions: LineClassification['dimension'][] = ['ACCOUNT', 'VAT_TREATMENT', 'VAT_DEDUCTIBILITY', 'EXPENSE_TAX_TREATMENT']

export function ClassificationWorkspace({ invoice }: { invoice: Invoice }) {
  if (!hasReachedClassification(invoice)) {
    return <div className="card p-10 text-center"><Clock3 className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Clasificarea nu este încă disponibilă</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Factura trebuie să ajungă la etapa de clasificare înainte ca aceste date să fie afișate.</p></div>
  }

  const dimensions = invoice.modelVersion === 'ACCOUNTING_DOMAIN_V2' ? domainDimensions : legacyDimensions
  const uncertainItems = invoice.task?.type === 'CLASSIFICATION' ? invoice.task.classificationItems ?? [] : []
  return (
    <div className="space-y-5">
      {invoice.readinessReason && <div role="alert" className="card p-4 text-sm text-[var(--warning)]">{invoice.readinessReason}</div>}
      {invoice.accountingSnapshot?.profile && <p className="text-xs">Profil fiscal v{invoice.accountingSnapshot.profile.version} · Politică: {invoice.accountingSnapshot.profile.chartPolicy} · {invoice.accountingSnapshot.pack?.testOnly ? "Configurație sintetică TEST_ONLY" : "Configurație contabilă"}</p>}
      {invoice.accountingSnapshot?.profile && <div className="card grid gap-2 p-4 text-xs sm:grid-cols-2">
        <p>Regim fiscal: {profileLabel(invoice.accountingSnapshot.profile.taxRegime)}</p><p>Înregistrare TVA: {profileLabel(invoice.accountingSnapshot.profile.vatRegistration)}</p>
        <p>Activitate cu drept de deducere: {profileLabel(invoice.accountingSnapshot.profile.deductionActivity)}</p><p>TVA la încasare client: {profileLabel(invoice.accountingSnapshot.profile.cashAccounting)} · Pro-rata: {profileLabel(invoice.accountingSnapshot.profile.proRata)}</p>
        <p>Profil valabil: {invoice.accountingSnapshot.profile.effectiveFrom ?? 'dată neconfirmată'} — {invoice.accountingSnapshot.profile.effectiveTo ?? 'fără dată finală'}</p>
      </div>}
      {invoice.modelVersion === 'ACCOUNTING_DOMAIN_V2' && invoice.sourceFacts && <div className="card grid gap-2 p-4 text-xs sm:grid-cols-2">
        <p>ID fiscal furnizor din sursă: {invoice.sourceFacts.supplierVatId ?? 'absent'} · Țară: {invoice.sourceFacts.supplierCountry ?? 'absentă'}</p>
        <p>ID fiscal cumpărător din sursă: {invoice.sourceFacts.buyerVatId ?? 'absent'} · Țară: {invoice.sourceFacts.buyerCountry ?? 'absentă'}</p>
        <p>TVA la încasare indicată în sursă: {profileLabel(invoice.sourceFacts.cashAccounting)}</p><p>Data exigibilității din sursă: {invoice.sourceFacts.taxPointDate ?? 'absentă'}</p>
        <p>Perioada prestației din sursă: {invoice.sourceFacts.periodStart ?? 'absentă'} — {invoice.sourceFacts.periodEnd ?? 'absentă'}</p>
      </div>}
      {invoice.task?.type === 'CLASSIFICATION' && <ClassificationTask invoice={invoice} />}
      <section className="card overflow-hidden">
        <div className="border-b border-[var(--border)] px-5 py-4"><h3 className="font-bold">Clasificări pe linii</h3><p className="mt-1 text-xs text-[var(--text-muted)]">Dimensiunile sigure sunt vizibile, dar nu necesită acțiune.</p></div>
        <div className="divide-y divide-[var(--border)]">
          {invoice.lines.map((line) => (
            <article key={line.id} className="p-5">
              <div className="flex items-center gap-3"><span className="grid size-7 place-items-center rounded-lg bg-[var(--surface-subtle)] text-xs font-bold">{line.position}</span><div><h4 className="text-sm font-bold">{line.description}</h4><div className="mt-0.5 text-xs text-[var(--text-muted)]">{line.quantity} {line.unit} · {formatMoney(line.netValue.amount, line.netValue.currency)} · Cotă TVA declarată în factură: {line.vatLabel} · {formatMoney(line.vatValue.amount, line.vatValue.currency)}</div></div></div>
              <div className="mt-2 text-xs text-[var(--text-muted)]">{line.sourceFacts && <>Categorie sursă: {line.sourceFacts.code || "absentă"} · Valoare TVA: {line.sourceFacts.vatOrigin === "CALCULATED" ? "calculată de Diana" : line.sourceFacts.vatOrigin === "DECLARED" ? "declarată în sursă" : "origine necunoscută"}{line.sourceFacts.exemptionReason && ` · ${line.sourceFacts.exemptionReason}`}</>}</div>
              <div className="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
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
      <div className="mt-3 text-sm font-bold">{displayDomainValue(uncertain?.typedValue ?? uncertain?.proposedTypedValue ?? confident?.typedValue, value)}</div>
      {confidence && <div className="mt-1 text-[11px] text-[var(--text-secondary)]">Încredere: {confidence}</div>}
      {explanation && <p className="mt-3 text-xs leading-5 text-[var(--text-secondary)]">{explanation}</p>}
      {legalBasis && <p className="mt-3 border-t border-[var(--border)] pt-3 text-[10px] leading-4 text-[var(--text-muted)]">{legalBasis}</p>}
      {rule && <p className="mt-2 text-xs">{rule.productionEligible ? "Regulă verificată pentru producție" : "Regulă demo / neverificată"}{rule.effectiveFrom && ` · ${rule.effectiveFrom} — ${rule.effectiveTo ?? "fără dată finală"}`}</p>}
      {rule && <Link to={`/rules/${rule.ruleId}?version=${rule.version}&returnTo=${encodeURIComponent(`/invoices/${invoiceId}?tab=classification`)}`} className="mt-3 block rounded-lg border border-[var(--border)] bg-[var(--surface)] p-2 text-[10px] font-semibold text-[var(--accent)] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Origine regulă: {rule.origin === 'GLOBAL' ? 'Regulă globală' : 'Override client'} · {rule.reference} · v{rule.version}</Link>}
      <Badge className="mt-3" tone={pending ? 'warning' : 'success'}>{pending ? 'De revizuit' : uncertain?.status === 'CORRECTED' ? 'Corectat' : (uncertain?.humanReviewed ?? confident?.humanReviewed) ? 'Confirmat de contabil' : 'Acceptat automat'}</Badge>
    </div>
  )
}

function hasReachedClassification(invoice: Invoice) {
  const classificationIndex = invoice.pipelinePath.indexOf('CLASSIFIED')
  const currentIndex = invoice.pipelinePath.indexOf(invoice.pipelineStatus)
  return classificationIndex >= 0 && currentIndex >= classificationIndex
}

function formatMoney(amount: number, currency: string) { return new Intl.NumberFormat('ro-RO', { style: 'currency', currency }).format(amount) }

function profileLabel(value?: string) { return ({ PROFIT_TAX: 'Impozit pe profit', MICROENTERPRISE: 'Microîntreprindere', ORDINARY_REGISTERED: 'Înregistrare obișnuită', NOT_REGISTERED: 'Neînregistrat', SPECIAL_REGISTERED: 'Înregistrare specială', WITH_DEDUCTION_RIGHT: 'Cu drept de deducere', MIXED: 'Activitate mixtă', WITHOUT_DEDUCTION_RIGHT: 'Fără drept de deducere', YES: 'Da', NO: 'Nu', UNKNOWN: 'Necunoscut', OTHER: 'Alt regim' } as Record<string, string>)[value ?? 'UNKNOWN'] ?? 'Necunoscut' }
