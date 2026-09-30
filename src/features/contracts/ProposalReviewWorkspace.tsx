import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { CommercialRule, ContractDocument, ProposedCommercialClause, ReviewedContract, ReviewedServiceTerm } from '../../repositories/invoiceRepository'
import { useConfirmContractDocument } from './contract-ingestion-hooks'
import { buildReviewItems, confirmationBlockers, documentRoleLabels, initialReviewValues, manualServiceTerm, reviewFieldInputs, reviewProgress, type ReviewDecision, type ReviewItem, type ReviewSignal } from './contract-review-model'
import { RuleExplanation } from './contract-review-parts'
import { ContractReviewWorkspace, Kbd } from './ContractReviewWorkspace'
import { useContractPdf } from './useContractPdf'

type Snapshot = { id: string; values: ReviewedContract; decisions: Record<string, ReviewDecision> }

const signalColor: Record<ReviewSignal['tone'], string> = { ok: 'border-[var(--success-border)] text-[var(--success)]', attention: 'border-[var(--warning-border)] text-[var(--warning)]', neutral: 'border-[var(--border)] text-[var(--text-secondary)]' }
const inputClass = 'mt-1 h-10 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm font-normal'

// Items the reviewer ticks as read correctly, and items with a value to edit.
const checkable = (item: ReviewItem) => Boolean(item.decisionKey) && (item.kind.type === 'field' || item.kind.type === 'service')
const editable = (item: ReviewItem) => item.kind.type === 'field' || item.kind.type === 'service'
// Fields the contract may simply not state.
const canBeMissing = (item: ReviewItem) => item.kind.type === 'field' && item.kind.key in reviewFieldInputs

// A document still in review, next to its PDF. Items the AI read with high
// confidence, found exactly in the PDF and stating their value come ticked;
// the reviewer checks the rest, one at a time, and confirms the contract when
// nothing is left. The reviewer's decisions stay in this page until then.
export function ProposalReviewWorkspace({ document, onStale, footer }: { document: ContractDocument; onStale: () => void; footer: ReactNode }) {
  const [params, setParams] = useSearchParams()
  const confirm = useConfirmContractDocument(document.clientId)
  const pdf = useContractPdf(document.clientId, document.id)
  const proposal = document.extraction?.proposal
  const [values, setValues] = useState<ReviewedContract>(() => initialReviewValues(proposal))
  const [decisions, setDecisions] = useState<Record<string, ReviewDecision>>({})
  const [editing, setEditing] = useState<Snapshot>()
  const key = useMemo(() => crypto.randomUUID(), [document.id, document.revision])
  const [stale, setStale] = useState(false)
  const unconfirmed = (proposal?.commercialClauses ?? []).filter((clause) => !values.commercialRules.some((rule) => rule.id === clause.rule?.id))
  const blockers = confirmationBlockers(values, document, unconfirmed.length)
  const { ready, scanned, locate } = pdf
  const items = useMemo(() => buildReviewItems(document, values, decisions, { ready, scanned, locate }), [document, values, decisions, ready, scanned, locate])
  const { verified, total, remaining } = reviewProgress(items)

  const jump = useCallback((id: string) => setParams((current) => { const next = new URLSearchParams(current); next.set('element', id); return next }, { replace: true }), [setParams])
  const nextAfter = (id: string | null) => {
    const index = items.findIndex((item) => item.id === id)
    return [...items.slice(index + 1), ...items.slice(0, Math.max(index, 0))].find((item) => item.tone === 'attention' && item.id !== id)
  }

  // Once the PDF text is read, open the first item to check, unless a link
  // already chose one. Nothing turns under the reviewer's hand afterwards.
  const opened = useRef(false)
  useEffect(() => {
    if (!ready || opened.current) return
    opened.current = true
    const first = items.find((item) => item.tone === 'attention')
    if (!params.get('element') && first) jump(first.id)
  }, [ready, items, params, jump])

  const decide = (decisionKey: string | undefined, decision: ReviewDecision) => { if (decisionKey) setDecisions((current) => ({ ...current, [decisionKey]: decision })) }
  const moveOn = (item: ReviewItem) => { const next = nextAfter(item.id); if (next) jump(next.id) }
  const markChecked = (item: ReviewItem) => { decide(item.decisionKey, 'CHECKED'); setEditing(undefined); moveOn(item) }
  const startEdit = (item: ReviewItem) => setEditing({ id: item.id, values, decisions })
  const cancelEdit = () => { if (!editing) return; setValues(editing.values); setDecisions(editing.decisions); setEditing(undefined) }
  // Typing in a value opens it for editing, so the editor stays while the
  // value becomes valid and Esc restores what was there before.
  const editIn = (item: ReviewItem) => { if (editing?.id !== item.id) startEdit(item) }
  const setField = (item: ReviewItem, patch: Partial<ReviewedContract>) => { editIn(item); setValues((current) => ({ ...current, ...patch })); decide(item.decisionKey, 'CHECKED') }
  const setFieldValue = (item: ReviewItem, fieldKey: keyof ReviewedContract, value: string) => { editIn(item); setValues((current) => ({ ...current, [fieldKey]: value })); decide(item.decisionKey, 'CHECKED') }
  const updateTerm = (item: ReviewItem, index: number, patch: Partial<ReviewedServiceTerm>) => { editIn(item); setValues((current) => ({ ...current, serviceTerms: current.serviceTerms.map((term, position) => position === index ? { ...term, ...patch } : term) })); decide(item.decisionKey, 'CHECKED') }
  // The contract does not state the value: it is left empty and ticked.
  const markMissing = (item: ReviewItem) => {
    if (item.kind.type !== 'field') return
    const fieldKey = item.kind.key
    setValues((current) => ({ ...current, [fieldKey]: '' }))
    markChecked(item)
  }
  const removeTerm = (index: number) => { setValues((current) => ({ ...current, serviceTerms: current.serviceTerms.filter((_, position) => position !== index) })); setEditing(undefined) }
  const addTerm = () => {
    setEditing({ id: `service:${values.serviceTerms.length}`, values, decisions })
    setValues((current) => ({ ...current, serviceTerms: [...current.serviceTerms, manualServiceTerm(current.currency)] }))
    jump(`service:${values.serviceTerms.length}`)
  }
  const includeRule = (item: ReviewItem, rule: CommercialRule) => { setValues((current) => ({ ...current, commercialRules: [...current.commercialRules, rule] })); moveOn(item) }
  const deferRule = (item: ReviewItem) => { decide(item.decisionKey, 'DEFERRED'); moveOn(item) }
  // Removing an included rule leaves it for after confirmation, and the
  // contract cannot be complete without it.
  const removeRule = (item: ReviewItem, ruleId: string) => { setValues((current) => ({ ...current, commercialRules: current.commercialRules.filter((rule) => rule.id !== ruleId), coverage: 'PARTIAL' })); decide(item.decisionKey, 'DEFERRED') }

  // The confirmed page replaces this one once the document is read again, so
  // the jump to the first clause left to settle follows the mutation itself.
  const submit = () => confirm.mutateAsync({ document, contract: values, key }).then(
    () => jump('group:clauses'),
    (error) => { if ((error as { response?: { status?: number } }).response?.status === 409) { setStale(true); onStale() } },
  )

  const onShortcut = (event: KeyboardEvent, item: ReviewItem) => {
    if (event.key === 'Escape') { if (editing) { event.preventDefault(); cancelEdit() } return }
    const target = event.target instanceof HTMLElement ? event.target : null
    // Enter on a button other than a row presses that button.
    if (target?.closest('button,a,summary') && !target.closest('[data-review-row]')) return
    if (event.key === 'Enter') {
      event.preventDefault()
      if (checkable(item)) markChecked(item)
      else moveOn(item)
    } else if ((event.key === 'e' || event.key === 'E') && editable(item)) {
      event.preventDefault()
      startEdit(item)
    }
  }

  const current = params.get('element')
  const confirmButton = !ready
    ? <Button type="button" disabled>Se citește PDF-ul…</Button>
    : remaining.length
      ? <Button type="button" onClick={() => jump((nextAfter(current) ?? remaining[0]).id)}>Mai ai {remaining.length} de verificat</Button>
      : <Button type="button" disabled={confirm.isPending || stale} onClick={() => void submit()}>{confirm.isPending ? 'Se confirmă…' : 'Confirmă contractul'}</Button>

  return (
    <ContractReviewWorkspace
      document={document}
      items={items}
      pdf={pdf}
      eyebrow="Propunere AI · de revizuit"
      title={document.originalFilename}
      subtitle={<><span>{document.clientName ?? document.clientId}</span><Progress ready={ready} verified={verified} total={total} /></>}
      badges={<Badge tone="info">De revizuit</Badge>}
      actions={confirmButton}
      nextLabel={() => 'Următorul de verificat'}
      onShortcut={onShortcut}
      keyHints={<> · <Kbd>Enter</Kbd> corect · <Kbd>E</Kbd> corectează · <Kbd>Esc</Kbd> renunță</>}
      groupActions={{ services: <Button type="button" variant="secondary" size="sm" onClick={addTerm}>Adaugă serviciu</Button> }}
      notice={<>
        {blockers.length > 0 && <div role="alert" className="card border-[var(--danger-border)] px-5 py-3"><p className="text-sm font-semibold">Confirmarea mai are nevoie de:</p><ul className="mt-1 list-disc pl-5 text-sm">{blockers.map((blocker) => <li key={`${blocker.itemId}:${blocker.message}`}><button type="button" className="text-left underline decoration-dotted" onClick={() => jump(blocker.itemId)}>{blocker.message}</button></li>)}</ul></div>}
        {(confirm.isError || stale) && <p role="alert" className="text-sm text-[var(--danger)]">{stale ? 'Extragerea s-a modificat sau contractul a fost deja confirmat. Reîncarcă propunerea înainte de a confirma.' : 'Serverul a respins confirmarea. Verifică blocajele afișate.'}</p>}
      </>}
      renderDetail={(item) => (
        <ItemDetail item={item} values={values} unconfirmed={unconfirmed.length} editing={editing?.id === item.id}
          onChecked={() => markChecked(item)} onEdit={() => startEdit(item)} onCancel={cancelEdit}
          onMissing={() => markMissing(item)}
          onField={(patch) => setField(item, patch)} onValue={(fieldKey, value) => setFieldValue(item, fieldKey, value)} onTerm={(index, patch) => updateTerm(item, index, patch)} onRemoveTerm={removeTerm}
          onInclude={(rule) => includeRule(item, rule)} onDefer={() => deferRule(item)} onRemoveRule={(ruleId) => removeRule(item, ruleId)} />
      )}
      footer={footer}
    />
  )
}

function Progress({ ready, verified, total }: { ready: boolean; verified: number; total: number }) {
  if (!ready) return <span role="status" className="mt-1 block text-xs">Se citește PDF-ul…</span>
  const left = total - verified
  return (
    <span className="mt-1.5 flex items-center gap-3 text-xs">
      <span aria-hidden="true" className="h-1.5 w-40 overflow-hidden rounded-full bg-[var(--surface-subtle)]"><span className="block h-full bg-[var(--success)]" style={{ width: `${total ? (verified / total) * 100 : 100}%` }} /></span>
      <span role="status">{verified} din {total} verificate{left ? ` · ${left} de verificat` : ' · nimic de verificat'}</span>
    </span>
  )
}

interface DetailProps {
  item: ReviewItem
  values: ReviewedContract
  unconfirmed: number
  editing: boolean
  onChecked: () => void
  onEdit: () => void
  onCancel: () => void
  onMissing: () => void
  onField: (patch: Partial<ReviewedContract>) => void
  onValue: (fieldKey: keyof ReviewedContract, value: string) => void
  onTerm: (index: number, patch: Partial<ReviewedServiceTerm>) => void
  onRemoveTerm: (index: number) => void
  onInclude: (rule: CommercialRule) => void
  onDefer: () => void
  onRemoveRule: (ruleId: string) => void
}

// The active row: why it is ticked or not, and what the reviewer can do.
function ItemDetail(props: DetailProps) {
  const { item } = props
  return (
    <>
      {!!item.signals?.length && <ul aria-label="Semnale" className="flex flex-wrap gap-1.5">{item.signals.map((signal) => <li key={signal.label} className={`rounded-full border px-2 py-0.5 text-xs font-semibold ${signalColor[signal.tone]}`}>{signal.label}</li>)}</ul>}
      <ItemActions {...props} />
    </>
  )
}

function ItemActions({ item, values, unconfirmed, editing, onChecked, onEdit, onCancel, onMissing, onField, onValue, onTerm, onRemoveTerm, onInclude, onDefer, onRemoveRule }: DetailProps) {
  const kind = item.kind
  if (kind.type === 'coverage') {
    return <label className="block text-xs font-semibold">Acoperire comercială<select aria-label="Acoperire comercială" value={values.coverage} disabled={unconfirmed > 0} onChange={(event) => onField({ coverage: event.target.value as ReviewedContract['coverage'] })} className={inputClass}><option value="COMPLETE">Completă</option><option value="PARTIAL">Parțială</option><option value="CONFLICTED">Cu conflicte</option></select>{unconfirmed > 0 && <span className="mt-1 block font-normal text-[var(--text-muted)]">Rămâne parțială cât timp o clauză propusă nu e inclusă în confirmare.</span>}</label>
  }
  if (kind.type === 'draft-clause') return <ClauseActions item={item} clause={kind.clause} status={kind.status} onInclude={onInclude} onDefer={onDefer} onRemoveRule={onRemoveRule} />
  if (kind.type !== 'field' && kind.type !== 'service') return null
  const manual = kind.type === 'service' && !item.decisionKey
  const open = editing || Boolean(item.blocker) || manual
  // Enter in a value keeps it and moves on; Esc restores the value.
  const onKeyDown = (event: ReactKeyboardEvent) => {
    const tag = (event.target as HTMLElement).tagName
    if (event.key === 'Escape') { event.preventDefault(); onCancel() }
    else if (event.key === 'Enter' && tag === 'INPUT') { event.preventDefault(); onChecked() }
  }
  return (
    <div className="space-y-3">
      {open && <div onKeyDown={onKeyDown}>{kind.type === 'field' ? <FieldEditor fieldKey={kind.key} values={values} focus={editing} onField={onField} onValue={onValue} /> : <ServiceEditor index={kind.index} term={values.serviceTerms[kind.index]} focus={editing} onTerm={onTerm} />}</div>}
      <div className="flex flex-wrap gap-2">
        {open
          ? <>{!manual && <Button type="button" size="sm" onClick={onChecked}>Gata <Kbd>↵</Kbd></Button>}{editing && <Button type="button" size="sm" variant="secondary" onClick={onCancel}>Renunță</Button>}</>
          : <><Button type="button" size="sm" onClick={onChecked}>Corect <Kbd>↵</Kbd></Button><Button type="button" size="sm" variant="secondary" onClick={onEdit}>Corectează</Button>{canBeMissing(item) && <Button type="button" size="sm" variant="secondary" onClick={onMissing}>Lipsă</Button>}</>}
        {kind.type === 'service' && <Button type="button" size="sm" variant="secondary" onClick={() => onRemoveTerm(kind.index)}>Elimină</Button>}
      </div>
    </div>
  )
}

function FieldEditor({ fieldKey, values, focus, onField, onValue }: { fieldKey: keyof ReviewedContract; values: ReviewedContract; focus: boolean; onField: (patch: Partial<ReviewedContract>) => void; onValue: (fieldKey: keyof ReviewedContract, value: string) => void }) {
  if (fieldKey === 'periodType') {
    return (
      <fieldset className="text-xs">
        <legend className="font-semibold">Durata contractului</legend>
        <div className="mt-1 flex gap-4"><label><input type="radio" checked={values.periodType === 'FIXED_TERM'} onChange={() => onField({ periodType: 'FIXED_TERM' })} /> Determinată</label><label><input type="radio" checked={values.periodType === 'INDEFINITE_TERM'} onChange={() => onField({ periodType: 'INDEFINITE_TERM', effectiveTo: '' })} /> Nedeterminată</label></div>
        {values.periodType === 'FIXED_TERM' && <label className="mt-2 block font-semibold">Data de sfârșit<input aria-label="Data de sfârșit" type="date" autoFocus={focus} value={values.effectiveTo} onChange={(event) => onField({ effectiveTo: event.target.value })} className={inputClass} /></label>}
      </fieldset>
    )
  }
  if (fieldKey === 'documentRole') {
    return <label className="block text-xs font-semibold">Rol document<select aria-label="Rol document" autoFocus={focus} value={values.documentRole} onChange={(event) => onField({ documentRole: event.target.value as ReviewedContract['documentRole'] })} className={inputClass}>{Object.entries(documentRoleLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
  }
  const input = reviewFieldInputs[fieldKey]
  if (!input) return null
  return <label className="block text-xs font-semibold">{input.label}<input aria-label={input.label} type={input.type ?? 'text'} autoFocus={focus} value={String(values[fieldKey] ?? '')} onChange={(event) => onValue(fieldKey, fieldKey === 'currency' ? event.target.value.toUpperCase() : event.target.value)} className={inputClass} /></label>
}

function ServiceEditor({ index, term, focus, onTerm }: { index: number; term: ReviewedServiceTerm | undefined; focus: boolean; onTerm: (index: number, patch: Partial<ReviewedServiceTerm>) => void }) {
  if (!term) return null
  const update = (patch: Partial<ReviewedServiceTerm>) => onTerm(index, patch)
  const n = index + 1
  return (
    <div className="grid gap-2 text-xs sm:grid-cols-2">
      <label className="font-semibold sm:col-span-2">Serviciu<input aria-label={`Serviciu ${n}`} autoFocus={focus} value={term.serviceDescription} onChange={(event) => update({ serviceDescription: event.target.value })} className={inputClass} /></label>
      <label className="font-semibold">Model tarifare<select aria-label={`Model tarifare ${n}`} value={term.pricingModel} onChange={(event) => update({ pricingModel: event.target.value as ReviewedServiceTerm['pricingModel'] })} className={inputClass}><option value="">Selectează</option><option value="FIXED_FEE">Tarif fix</option><option value="UNIT_RATE">Tarif unitar</option><option value="FIXED_TOTAL">Valoare totală</option></select></label>
      <label className="font-semibold">Periodicitate<select aria-label={`Periodicitate ${n}`} value={term.billingFrequency} onChange={(event) => update({ billingFrequency: event.target.value as ReviewedServiceTerm['billingFrequency'] })} className={inputClass}><option value="UNKNOWN">Nespecificată</option><option value="MONTHLY">Lunar</option><option value="QUARTERLY">Trimestrial</option><option value="ANNUAL">Anual</option><option value="PER_OCCURRENCE">Per prestație</option></select></label>
      <label className="font-semibold">Preț<input aria-label={`Preț ${n}`} value={term.unitPrice} onChange={(event) => update({ unitPrice: event.target.value })} className={inputClass} /></label>
      <label className="font-semibold">Monedă<input aria-label={`Monedă serviciu ${n}`} value={term.currency} onChange={(event) => update({ currency: event.target.value.toUpperCase() })} className={inputClass} /></label>
      {term.pricingModel === 'UNIT_RATE' && <>
        <label className="font-semibold">Unitate<input aria-label={`Unitate ${n}`} placeholder="ex. SALARIAT" value={term.unit} onChange={(event) => update({ unit: event.target.value })} className={inputClass} /></label>
        <label className="font-semibold">Cantitate / determinant<input aria-label={`Determinant ${n}`} value={term.quantityDriver} onChange={(event) => update({ quantityDriver: event.target.value })} className={inputClass} /></label>
      </>}
    </div>
  )
}

function ClauseActions({ item, clause, status, onInclude, onDefer, onRemoveRule }: { item: ReviewItem; clause: ProposedCommercialClause; status: string; onInclude: (rule: CommercialRule) => void; onDefer: () => void; onRemoveRule: (ruleId: string) => void }) {
  if (status === 'RECOGNIZED') return <p className="text-xs text-[var(--text-secondary)]">Diana a citit valoarea direct din clauză și o confirmă automat odată cu contractul. O poți modifica după confirmare.</p>
  if (status === 'AFTER_CONFIRM') return <p className="text-xs text-[var(--text-secondary)]">Clauza nu are o formulă executabilă, deci nu validează automat facturile. După confirmare o completezi, o confirmi ca regulă sau o închizi cu motiv; până atunci acoperirea contractului rămâne parțială. Nu blochează confirmarea.</p>
  return (
    <div className="space-y-2">
      <RuleExplanation rule={clause.rule} />
      {status === 'INCLUDED'
        ? <Button type="button" size="sm" variant="secondary" onClick={() => onRemoveRule(clause.rule.id)}>Elimină din confirmare</Button>
        : <div className="flex flex-wrap gap-2"><Button type="button" size="sm" onClick={() => onInclude(clause.rule)}>Confirmă regula</Button>{status !== 'DEFERRED' && <Button type="button" size="sm" variant="secondary" onClick={onDefer}>Lasă pentru după confirmare</Button>}</div>}
      {item.tone !== 'attention' && status === 'DEFERRED' && <p className="text-xs text-[var(--text-muted)]">Nicio regulă propusă nu validează facturi înainte de confirmarea ta; o poți confirma și din pagina contractului confirmat.</p>}
    </div>
  )
}
