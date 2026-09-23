import { useState } from 'react'
import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '../auth/auth-api'
import { Button } from '../../components/ui/button'
import type { Invoice } from '../../domain/invoice'

type Entry = { phase: string; debitAccount: string; creditAccount: string; amount: string; currency: string; invoiceLineIds: string[]; explanation: string }
type Treatment = { invoiceLineId: string; vatKind: string; vatRate?: string; vatBase: string; vatAmount: string; vatTiming: string; vatAccount?: string; vatDeductibility: string; expenseDeductibility: string; deductibilityCondition?: string }
type Proposal = { schemaVersion: string; clientId: string; invoiceId: string; invoiceRevision: number; entries: Entry[]; lineTreatments: Treatment[]; citations: { fragmentId: string; versionId: string; citationKey: string; contentHash: string }[]; reasoningSummary: string; confidence: string; source: string; requiresReview: boolean }
type Run = { id: string; status: string; model: string; invoiceRevision: number; proposal?: Proposal; validationIssues: { Code: string; Path: string; Message: string }[]; review?: { Action: string; Reason: string; ActorDisplay: string; FinalDecision?: Proposal } }

function endpoint(invoice: Invoice) { return `/clients/${encodeURIComponent(invoice.clientId)}/invoices/${encodeURIComponent(invoice.id)}/accounting-analysis` }
function message(error: unknown) { return axios.isAxiosError(error) ? (error.response?.data?.message ?? error.message) : error instanceof Error ? error.message : 'Operațiunea a eșuat.' }

export function AccountingAnalysisCard({ invoice }: { invoice: Invoice }) {
  const queryClient = useQueryClient()
  const key = ['accounting-analysis', invoice.clientId, invoice.id]
  const [reason, setReason] = useState('')
  const [draft, setDraft] = useState('')
  const [editing, setEditing] = useState(false)
  const [localError, setLocalError] = useState('')
  const analysis = useQuery({
    queryKey: key,
    queryFn: async () => { try { return (await apiClient.get<Run>(endpoint(invoice))).data } catch (error) { if (axios.isAxiosError(error) && error.response?.status === 404) return null; throw error } },
    refetchInterval: query => query.state.data?.status === 'RUNNING' ? 2000 : false,
  })
  const request = useMutation({ mutationFn: async () => (await apiClient.post<Run>(endpoint(invoice), {}, { headers: { 'Idempotency-Key': crypto.randomUUID() } })).data, onSuccess: run => queryClient.setQueryData(key, run) })
  const review = useMutation({ mutationFn: async (body: { analysisId: string; action: 'APPROVE' | 'EDIT' | 'REJECT'; reason: string; finalDecision?: Proposal }) => (await apiClient.post<Run>(`${endpoint(invoice)}/review`, body, { headers: { 'Idempotency-Key': crypto.randomUUID() } })).data, onSuccess: run => { queryClient.setQueryData(key, run); setEditing(false) } })
  const run = analysis.data
  const proposal = run?.review?.FinalDecision ?? run?.proposal
  const submit = (action: 'APPROVE' | 'EDIT' | 'REJECT') => {
    if (!run) return
    setLocalError('')
    let finalDecision: Proposal | undefined
    if (action === 'EDIT') {
      try { finalDecision = JSON.parse(draft) as Proposal; if (!finalDecision || !Array.isArray(finalDecision.entries)) throw new Error('JSON invalid') }
      catch { setLocalError('Corectează JSON-ul propunerii înainte de salvare.'); return }
    }
    review.mutate({ analysisId: run.id, action, reason: reason.trim(), finalDecision })
  }
  return <section className="card p-5" aria-labelledby="accounting-analysis-title">
    <div className="flex flex-wrap items-start justify-between gap-3"><div><p className="eyebrow">TEST_ONLY · mediu local</p><h3 id="accounting-analysis-title" className="mt-1 font-bold">Analiză contabilă asistată</h3></div><span className="rounded-full border border-[var(--warning-border)] px-3 py-1 text-xs font-semibold">{run?.review ? `Review: ${run.review.Action}` : run?.status ?? 'Nerulată'}</span></div>
    <p className="mt-2 text-sm text-[var(--text-secondary)]">Propunere experimentală bazată pe corpusul local. Nu postează note contabile și nu modifică exportul SAGA.</p>
    {analysis.isLoading && <p className="mt-3 text-sm">Se încarcă analiza…</p>}
    {analysis.isError && <p role="alert" className="mt-3 text-sm text-[var(--danger)]">{message(analysis.error)}</p>}
    {!run && !analysis.isLoading && !analysis.isError && <Button className="mt-4" type="button" disabled={request.isPending} onClick={() => request.mutate()}>Generează propunerea</Button>}
    {run?.status === 'RUNNING' && <p className="mt-3 text-sm" role="status">Analiza se procesează în Asynq…</p>}
    {run?.status === 'VALIDATION_FAILED' && <p className="mt-3 text-sm text-[var(--danger)]">Propunerea nu a trecut validările deterministe. Nu poate fi aprobată.</p>}
    {run?.status === 'PROVIDER_FAILED' && <p className="mt-3 text-sm text-[var(--danger)]">Furnizorul AI nu a putut genera o propunere validă.</p>}
    {!!run?.validationIssues?.length && <ul className="mt-3 list-inside list-disc text-xs text-[var(--danger)]">{run.validationIssues.map((issue, index) => <li key={index}>{issue.Code}: {issue.Message} ({issue.Path})</li>)}</ul>}
    {proposal && <div className="mt-4 space-y-4"><p className="text-sm">{proposal.reasoningSummary}</p><p className="text-xs text-[var(--text-muted)]">Model: {run?.model} · încredere: {proposal.confidence} · revizia facturii: {run?.invoiceRevision}</p>
      <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="border-b"><th className="p-2">Fază</th><th className="p-2">Debit</th><th className="p-2">Credit</th><th className="p-2">Sumă</th><th className="p-2">Explicație</th></tr></thead><tbody>{proposal.entries.map((entry, index) => <tr className="border-b" key={index}><td className="p-2">{entry.phase}</td><td className="p-2">{entry.debitAccount}</td><td className="p-2">{entry.creditAccount}</td><td className="p-2">{entry.amount} {entry.currency}</td><td className="p-2">{entry.explanation}</td></tr>)}</tbody></table></div>
      <div><h4 className="text-sm font-semibold">TVA și deductibilitate pe linii</h4><ul className="mt-1 space-y-1 text-xs">{proposal.lineTreatments.map(item => <li key={item.invoiceLineId}>{item.invoiceLineId}: TVA {item.vatAmount} ({item.vatDeductibility}), cheltuială {item.expenseDeductibility}</li>)}</ul></div>
      <details className="text-xs"><summary>Temeiuri citate</summary><ul className="mt-2 list-inside list-disc">{proposal.citations.map(citation => <li key={citation.fragmentId}>{citation.citationKey} · {citation.versionId} · hash {citation.contentHash.slice(0, 12)}…</li>)}</ul></details>
    </div>}
    {run?.status === 'PROPOSED' && !run.review && <div className="mt-5 space-y-3 border-t pt-4"><label className="block text-xs">Motiv / notă de review<textarea value={reason} onChange={event => setReason(event.target.value)} className="mt-1 min-h-16 w-full rounded-lg border p-2 text-sm" /></label><div className="flex flex-wrap gap-2"><Button type="button" disabled={review.isPending} onClick={() => submit('APPROVE')}>Aprobă propunerea</Button><Button type="button" variant="secondary" disabled={review.isPending} onClick={() => { setDraft(JSON.stringify(run.proposal, null, 2)); setEditing(value => !value) }}>Editează JSON</Button><Button type="button" variant="danger" disabled={review.isPending || !reason.trim()} onClick={() => submit('REJECT')}>Respinge</Button></div>{editing && <div><p className="mb-2 text-xs text-[var(--warning)]">Modifică doar câmpurile necesare; serverul verifică din nou toate conturile, sumele, liniile și citările.</p><textarea aria-label="Propunere contabilă JSON" spellCheck={false} value={draft} onChange={event => setDraft(event.target.value)} className="min-h-80 w-full rounded-lg border p-3 font-mono text-xs" /><Button type="button" disabled={review.isPending || !reason.trim()} onClick={() => submit('EDIT')}>Salvează decizia editată</Button></div>}</div>}
    {(request.isError || review.isError || localError) && <p role="alert" className="mt-3 text-sm text-[var(--danger)]">{localError || message(request.error ?? review.error)}</p>}
    {run?.review && <p className="mt-3 text-xs text-[var(--text-secondary)]">{run.review.ActorDisplay}: {run.review.Reason || 'fără notă'}.</p>}
  </section>
}
