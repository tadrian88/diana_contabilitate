import { ArrowRight, TriangleAlert } from 'lucide-react'
import { useMemo } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import type { Invoice } from '../../domain/invoice'
import type { CommercialValidationRun } from '../../repositories/invoiceRepository'
import { buildCommercialView, type CommercialCheck } from './commercial-view'
import { useCommercialValidation } from './invoice-hooks'
import { actionTab, getUnresolvedIssueCount, type InvoiceTab } from './invoice-view'

interface Step { text: string; action: string; to: string }

const tabNames: Record<InvoiceTab, string> = { summary: 'Rezumat', contract: 'Contract', lines: 'Linii factură', classification: 'Clasificare', history: 'Istoric' }

// What the reviewer has to do on an invoice waiting for a person, each step
// linking straight to where it is done. Nothing is shown while Diana works
// on its own.
export function NextStepsCard({ invoice }: { invoice: Invoice }) {
  const [params] = useSearchParams()
  const { data: run } = useCommercialValidation(invoice)
  const tab = actionTab(invoice)
  const link = (target: InvoiceTab, extra: Record<string, string> = {}) => {
    const next = new URLSearchParams(params)
    next.delete('verifica')
    next.delete('asociere')
    next.set('tab', target)
    for (const [key, value] of Object.entries(extra)) next.set(key, value)
    return `?${next.toString()}`
  }
  if (tab === 'summary') return null
  const steps = invoice.pipelineStatus === 'AWAITING_COMMERCIAL_REVIEW' && run ? commercialSteps(invoice, run, link) : undefined
  const fallback: Step = { text: nextStepText(invoice), action: `Deschide ${tabNames[tab]}`, to: link(tab) }
  const items = steps?.length ? steps : [fallback]
  return (
    <section aria-labelledby="next-steps-title" className="rounded-xl border border-[var(--warning-border)] bg-[var(--warning-soft)] p-4">
      <h3 id="next-steps-title" className="flex items-center gap-2 text-sm font-bold"><TriangleAlert aria-hidden="true" className="size-4 text-[var(--warning)]" />Ce ai de făcut</h3>
      <ol className="mt-3 space-y-2">
        {items.map((step, index) => (
          <li key={step.text} className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-[var(--surface)] px-3 py-2.5 text-sm">
            <span>{items.length > 1 ? `${index + 1}. ` : ''}{step.text}</span>
            <Link to={step.to} className="inline-flex h-9 items-center gap-1.5 rounded-lg bg-[var(--accent)] px-3 text-xs font-semibold text-[var(--accent-contrast)]">{step.action}<ArrowRight aria-hidden="true" className="size-4" /></Link>
          </li>
        ))}
      </ol>
    </section>
  )
}

// The count shown on the tab where the invoice's next action lives.
export function useActionCount(invoice: Invoice | undefined) {
  const { data: run } = useCommercialValidation(invoice)
  return useMemo(() => {
    if (!invoice) return 0
    if (invoice.pipelineStatus === 'AWAITING_COMMERCIAL_REVIEW' && run) return buildCommercialView(run, invoice).pendingCount
    return getUnresolvedIssueCount(invoice)
  }, [invoice, run])
}

function commercialSteps(invoice: Invoice, run: CommercialValidationRun, link: (tab: InvoiceTab, extra?: Record<string, string>) => string): Step[] {
  const view = buildCommercialView(run, invoice)
  const steps: Step[] = []
  if (view.mappings.length) steps.push({ text: view.mappings.length === 1 ? 'O linie nu este asociată cu un serviciu din contract.' : `${view.mappings.length} linii nu sunt asociate cu un serviciu din contract.`, action: 'Asociază serviciile', to: link('contract', { asociere: 'toate' }) })
  const coverage = run.findings.find((finding) => finding.code === 'CONTRACT_COVERAGE_INCOMPLETE' && !finding.override)
  if (coverage) {
    const documentId = coverage.evidence?.find((item) => item.documentId)?.documentId ?? run.findings.flatMap((finding) => finding.evidence ?? []).find((item) => item.documentId)?.documentId
    steps.push({ text: 'Contractul are clauze „De rezolvat”: cât timp rămân deschise, toate facturile lui așteaptă verificare.', action: 'Deschide contractul', to: documentId ? `/contracts/documents/${encodeURIComponent(invoice.clientId)}/${encodeURIComponent(documentId)}?element=group:clauses` : link('contract') })
  }
  const mappedLines = new Set(view.mappings.map((mapping) => mapping.line.id))
  const listed = (check: CommercialCheck) => check.code === 'CONTRACT_COVERAGE_INCOMPLETE' || (check.code === 'SERVICE_LINE_UNCOVERED' && check.lineIds.some((id) => mappedLines.has(id)))
  const others = [...view.invoiceChecks, ...view.lineGroups.flatMap((group) => group.checks)].filter((check) => !listed(check) && check.outcome !== 'CONFORM' && !check.findings.every((finding) => finding.override))
  if (others.length) steps.push({ text: others.length === 1 ? `De verificat: ${others[0].title.toLocaleLowerCase('ro-RO')}.` : `${others.length} verificări au abateri sau date lipsă.`, action: others.length === 1 && others[0].kind === 'comparison' ? 'Verifică' : 'Deschide verificările', to: others.length === 1 && others[0].kind === 'comparison' ? link('contract', { verifica: others[0].id }) : link('contract') })
  return steps
}

function nextStepText(invoice: Invoice) {
  switch (invoice.pipelineStatus) {
    case 'AWAITING_CONTRACT': return 'Lipsește contractul furnizorului. Încarcă-l, solicită-l sau continuă fără contract, cu motiv; după asociere factura continuă automat.'
    case 'AWAITING_MATCH_CONFIRM': return 'Confirmă contractul recomandat sau alege altul.'
    case 'AWAITING_COMMERCIAL_REVIEW': return 'Verifică factura față de contract.'
    case 'AWAITING_REVIEW': return 'Revizuiește clasificările incerte.'
    default: return invoice.task?.reason ?? 'Factura așteaptă o acțiune.'
  }
}
