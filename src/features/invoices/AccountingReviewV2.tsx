import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { AlertTriangle, Check, CheckCircle2, ChevronDown, CircleHelp, Clock3, Edit3, RefreshCw, Scale, X } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import { useInvoiceRepository } from '../../app/repository-context'
import type { ClassificationReviewItem, Invoice, LineClassification, PromotionPreview, RuleCategory } from '../../domain/invoice'
import { CLASSIFICATION_DIMENSION_LABELS } from '../../domain/invoice'
import { AccountDecisionDialog } from './AccountDecisionDialog'
import { DomainCorrectionDialog } from './DomainCorrectionDialog'
import { displayDomainValue } from './domain-decision-view'
import { useApproveAllClassifications, useReviewClassification } from './invoice-mutations'
import { useReanalyzeClassification } from './invoice-hooks'

const dimensions: RuleCategory[] = ['ACCOUNT', 'VAT_TREATMENT', 'VAT_DEDUCTIBILITY', 'EXPENSE_TAX_TREATMENT']

type DecisionState = 'FINAL' | 'VALID_PROPOSAL' | 'INVALID' | 'UNRESOLVED' | 'REJECTED'
type Decision = { dimension: RuleCategory; final?: LineClassification; review?: ClassificationReviewItem; state: DecisionState }

export function AccountingReviewV2({ invoice }: { invoice: Invoice }) {
  const taskItems = invoice.task?.type === 'CLASSIFICATION' ? invoice.task.classificationItems ?? [] : []
  const decisions = useMemo(() => invoice.lines.flatMap(line => dimensions.map(dimension => decisionFor(line.classifications, taskItems, line.id, dimension))), [invoice.lines, taskItems])
  const counts = countDecisions(decisions)
  const attentionLineIds = new Set(invoice.lines.filter(line => dimensions.some(dimension => ['INVALID', 'UNRESOLVED', 'REJECTED'].includes(decisionFor(line.classifications, taskItems, line.id, dimension).state))).map(line => line.id))
  const [attentionOnly, setAttentionOnly] = useState(false)
  const visibleLines = attentionOnly ? invoice.lines.filter(line => attentionLineIds.has(line.id)) : invoice.lines

  return <div className="space-y-4">
    <WorkflowBanner invoice={invoice} validProposalCount={counts.valid} />
    <section className="card p-5" aria-labelledby="accounting-review-summary">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div><p className="eyebrow">Review contabil</p><h3 id="accounting-review-summary" className="mt-1 text-lg font-bold">Deciziile contabile ale facturii</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Verifică propunerile și corectează numai câmpurile care necesită atenție.</p></div>
        <div className="flex flex-wrap gap-2" aria-label="Sumar decizii"><SummaryBadge tone="success" value={counts.final} label="finale"/><SummaryBadge tone="warning" value={counts.valid} label="de verificat"/><SummaryBadge tone="danger" value={counts.problem} label="de rezolvat"/></div>
      </div>
      {invoice.accountingSnapshot?.profile && <p className="mt-3 text-xs text-[var(--text-muted)]">Profil fiscal v{invoice.accountingSnapshot.profile.version} · {profileLabel(invoice.accountingSnapshot.profile.taxRegime)} · {invoice.accountingSnapshot.pack?.testOnly ? 'Configurație sintetică TEST_ONLY' : 'Configurație contabilă activă'}</p>}
      {invoice.accountingWorkflowStatus === 'REVIEW_REQUIRED' && invoice.task?.reason && <p className="mt-2 text-xs text-[var(--text-secondary)]">{invoice.task.reason}</p>}
      {attentionLineIds.size > 0 && <label className="mt-4 inline-flex items-center gap-2 text-sm"><input type="checkbox" checked={attentionOnly} onChange={event => setAttentionOnly(event.target.checked)}/>Arată numai liniile care necesită atenție</label>}
    </section>

    <section aria-labelledby="invoice-accounting-lines" className="space-y-3">
      <h3 id="invoice-accounting-lines" className="sr-only">Clasificări pe liniile facturii</h3>
      {visibleLines.map(line => {
        const lineDecisions = dimensions.map(dimension => decisionFor(line.classifications, taskItems, line.id, dimension))
        const hasAttention = lineDecisions.some(item => item.state !== 'FINAL')
        return <details key={line.id} open={hasAttention || invoice.lines.length <= 10} className="card group overflow-hidden">
          <summary className="flex cursor-pointer list-none items-center justify-between gap-4 p-5 outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]">
            <div className="min-w-0"><div className="flex items-center gap-2"><span className="grid size-7 shrink-0 place-items-center rounded-lg bg-[var(--surface-subtle)] text-xs font-bold">{line.position}</span><h4 className="truncate text-sm font-bold">{line.description}</h4>{hasAttention && <Badge tone="warning">Necesită atenție</Badge>}</div><p className="mt-2 text-xs text-[var(--text-muted)]">{line.quantity} {line.unit} · {formatMoney(line.netValue.amount, line.netValue.currency)} + TVA {formatMoney(line.vatValue.amount, line.vatValue.currency)} · total {formatMoney(line.grossValue.amount, line.grossValue.currency)}{line.sourceFacts ? ` · TVA ${line.sourceFacts.vatOrigin === 'CALCULATED' ? 'calculată de Diana' : line.sourceFacts.vatOrigin === 'DECLARED' ? 'declarată în sursă' : 'cu origine neconfirmată'}` : ''}</p></div>
            <ChevronDown className="size-5 shrink-0 transition-transform group-open:rotate-180" aria-hidden="true"/>
          </summary>
          <div className="grid gap-3 border-t border-[var(--border)] bg-[var(--app-background)] p-4 lg:grid-cols-2 2xl:grid-cols-4">
            {lineDecisions.map(decision => <DecisionCard key={decision.dimension} decision={decision} invoice={invoice}/>) }
          </div>
        </details>
      })}
      {visibleLines.length === 0 && <div className="card p-8 text-center text-sm text-[var(--text-secondary)]">Nu există linii cu probleme de rezolvat.</div>}
    </section>

    <DerivedMonograph invoice={invoice}/>
  </div>
}

function WorkflowBanner({ invoice, validProposalCount }: { invoice: Invoice; validProposalCount: number }) {
  const reanalyze = useReanalyzeClassification(invoice)
  // A changed client context is a non-blocking warning: the accountant may
  // still decide manually or reanalyze with the current configuration.
  if (!invoice.classificationContext?.contextStale) return <WorkflowStatus invoice={invoice} validProposalCount={validProposalCount}/>
  return <div className="space-y-4">
    <StatusPanel tone="warning" title="Configurația clientului s-a modificat după această analiză." detail={`${staleReasons(invoice.classificationContext?.staleReasons ?? [])} Poți decide manual în continuare sau poți reanaliza factura cu configurația curentă.`}><Button disabled={reanalyze.isPending} onClick={() => reanalyze.mutate()}>{reanalyze.isPending ? 'Pornim reanalizarea…' : 'Reanalizează factura'}</Button>{reanalyze.isError && <p role="alert" className="text-sm text-[var(--danger)]">Reanalizarea nu a putut fi pornită.</p>}</StatusPanel>
    <WorkflowStatus invoice={invoice} validProposalCount={validProposalCount}/>
  </div>
}

function WorkflowStatus({ invoice, validProposalCount }: { invoice: Invoice; validProposalCount: number }) {
  const approveAll = useApproveAllClassifications(invoice.id)
  const reanalyze = useReanalyzeClassification(invoice)
  const [conflict, setConflict] = useState(false)
  // The stale-context banner already offers reanalysis; avoid a duplicate action.
  const offerReanalysis = !invoice.classificationContext?.contextStale
  const reanalyzeAction = offerReanalysis && <><Button variant="secondary" disabled={reanalyze.isPending} onClick={() => reanalyze.mutate()}>{reanalyze.isPending ? 'Pornim reanalizarea…' : 'Reanalizează factura'}</Button>{reanalyze.isError && <p role="alert" className="text-sm text-[var(--danger)]">Reanalizarea nu a putut fi pornită.</p>}</>
  const status = invoice.accountingWorkflowStatus
  const blocked = invoice.readinessReason === 'MISSING_FISCAL_PROFILE' || invoice.readinessReason === 'INVALID_ACCOUNTING_PROFILE'
  const failure = invoice.task?.reason?.toLowerCase().includes('asistată nu este disponibilă') || invoice.task?.reason?.toLowerCase().includes('automat') && invoice.task?.reason?.toLowerCase().includes('manual')

  if (invoice.readinessReason === 'MISSING_FISCAL_PROFILE') return <StatusPanel tone="warning" title="Profilul fiscal al clientului trebuie configurat înainte de analiza contabilă." detail="După ce salvezi profilul, reanalizează factura pentru a crea deciziile în contextul fiscal corect."><Link className="inline-flex rounded-lg border border-[var(--border)] px-4 py-2 text-sm font-semibold" to={`/clients/${encodeURIComponent(invoice.clientId)}#accounting-profile`}>Vezi profilul fiscal</Link><Button disabled={reanalyze.isPending} onClick={() => reanalyze.mutate()}>{reanalyze.isPending ? 'Pornim reanalizarea…' : 'Reanalizează factura'}</Button>{reanalyze.isError && <p role="alert" className="text-sm text-[var(--danger)]">Reanalizarea nu a putut fi pornită. Verifică dacă profilul este aplicabil la data facturii.</p>}</StatusPanel>
  if (invoice.readinessReason === 'INVALID_ACCOUNTING_PROFILE') return <StatusPanel tone="danger" title="Configurația contabilă a clientului trebuie corectată." detail={invoice.task?.reason || 'Înlocuiește conturile inexistente, inactive sau sintetice din profil înainte de reanalizare.'}><Link className="inline-flex rounded-lg bg-[var(--accent)] px-4 py-2 text-sm font-semibold text-white" to={`/clients/${encodeURIComponent(invoice.clientId)}#accounting-profile`}>Remediază profilul</Link></StatusPanel>
  if (status === 'APPLYING_RULES') return <StatusPanel title="Analizăm factura" detail="Aplicăm regulile și configurația contabilă a clientului. Nu este necesară nicio acțiune." busy/>
  if (status === 'AI_ANALYSIS_PENDING' || status === 'AI_ANALYSIS_RUNNING') return <StatusPanel title="Analiza contabilă este în curs" detail="Diana completează dimensiunile rămase. Pagina se actualizează automat, fără aprobări premature." busy/>
  if (status === 'COMPLETED') return <StatusPanel tone="success" title="Clasificare contabilă finalizată" detail="Toate deciziile obligatorii sunt finale. Monografia derivată poate fi consultată mai jos."/>
  if (failure) return <StatusPanel tone="warning" title="Analiza automată nu a putut fi finalizată." detail="Completează manual câmpurile rămase sau reanalizează factura după remedierea cauzei (de exemplu, după importul surselor legislative). Poți finaliza factura și fără reluarea analizei automate.">{reanalyzeAction}</StatusPanel>
  if (status === 'REVIEW_REQUIRED' || blocked) return <StatusPanel tone="warning" title="Factura necesită verificarea ta" detail={invoice.readinessReason || (validProposalCount ? 'Aprobarea în grup confirmă numai propunerile valide. Câmpurile invalide sau nerezolvate rămân deschise.' : 'Completează sau corectează câmpurile marcate înainte de finalizare.')}>
    {validProposalCount > 0 && <Button disabled={approveAll.isPending} onClick={() => { setConflict(false); approveAll.mutate(undefined, { onError: error => setConflict((error as {response?:{status?:number}}).response?.status === 409) }) }}>{approveAll.isPending ? 'Se aprobă…' : 'Aprobă toate propunerile valide'}</Button>}
    {conflict && <div role="alert" className="flex flex-wrap items-center gap-2 text-sm text-[var(--danger)]"><span>Factura a fost modificată între timp. Reîncarcă datele înainte de aprobare.</span><Button variant="secondary" onClick={() => window.location.reload()}><RefreshCw className="size-4"/>Reîncarcă</Button></div>}
    {approveAll.isError && !conflict && <p role="alert" className="text-sm text-[var(--danger)]">Propunerile nu au putut fi aprobate. Reîncarcă factura și verifică starea curentă.</p>}
    {reanalyzeAction}
  </StatusPanel>
  return <StatusPanel title="Clasificarea contabilă se pregătește" detail="Deciziile vor deveni disponibile după finalizarea etapelor anterioare." busy/>
}

function StatusPanel({ title, detail, tone = 'info', busy, children }: { title: string; detail: string; tone?: 'info'|'warning'|'danger'|'success'; busy?: boolean; children?: ReactNode }) {
  const colors = { info: 'border-[var(--info-border)] bg-[var(--info-soft)]', warning: 'border-[var(--warning-border)] bg-[var(--warning-soft)]', danger: 'border-[var(--danger-border)] bg-[var(--danger-soft)]', success: 'border-[var(--success-border)] bg-[var(--success-soft)]' }
  return <section className={`rounded-xl border p-5 ${colors[tone]}`} role={tone === 'danger' ? 'alert' : 'status'} aria-live="polite"><div className="flex items-start gap-3">{busy ? <Clock3 className="mt-0.5 size-5 animate-pulse"/> : tone === 'success' ? <CheckCircle2 className="mt-0.5 size-5"/> : tone === 'warning' || tone === 'danger' ? <AlertTriangle className="mt-0.5 size-5"/> : <CircleHelp className="mt-0.5 size-5"/>}<div className="flex-1"><h3 className="font-bold">{title}</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">{detail}</p>{children && <div className="mt-4 flex flex-wrap items-center gap-3">{children}</div>}</div></div></section>
}

function DecisionCard({ decision, invoice }: { decision: Decision; invoice: Invoice }) {
  const repository=useInvoiceRepository()
  const reviewMutation = useReviewClassification(invoice.id)
  const [editing, setEditing] = useState(false)
  const [accountEditing, setAccountEditing] = useState(false)
  const [rejecting, setRejecting] = useState(false)
  const [rejectReason, setRejectReason] = useState('')
  const [reusePreview,setReusePreview]=useState<PromotionPreview>()
  const [reuseBusy,setReuseBusy]=useState(false)
  const [reuseMessage,setReuseMessage]=useState('')
  const item = decision.review
  const record = item ?? decision.final
  const value = item?.typedValue ?? item?.proposedTypedValue ?? decision.final?.typedValue
  const fallback = item?.resolvedValue ?? item?.proposedValue ?? decision.final?.value
  const validProposal = decision.state === 'VALID_PROPOSAL'
  const canEdit = Boolean(item) && decision.state !== 'FINAL'
  const canReuse=decision.state==='FINAL'&&Boolean(record?.humanReviewed)&&Boolean(record?.revision)&&Boolean(invoice.revision)&&Boolean(invoice.currentClassificationRunId)&&!invoice.classificationContext?.contextStale
  const openReuse=async()=>{if(!record)return;setReuseBusy(true);setReuseMessage('');try{setReusePreview(await repository.previewApprovedKnowledge(invoice.clientId,invoice.id,record.id))}catch{setReuseMessage('Scope-ul nu mai poate fi pregătit. Reîncarcă factura.')}finally{setReuseBusy(false)}}
  const confirmReuse=async()=>{if(!reusePreview||!invoice.revision)return;setReuseBusy(true);setReuseMessage('');try{await repository.promoteApprovedKnowledge(invoice.clientId,invoice.id,reusePreview,invoice.revision);setReusePreview(undefined);setReuseMessage('Decizia este acum reutilizabilă pentru scope-ul confirmat.')}catch(error){const code=(error as {response?:{data?:{code?:string}}}).response?.data?.code;setReuseMessage(code==='KNOWLEDGE_DUPLICATE'?'Există deja o decizie reutilizabilă identică.':code==='KNOWLEDGE_CONFLICT'?'Acest scope are deja o valoare diferită și necesită rezolvare explicită.':'Factura sau decizia s-a modificat. Reîncarcă înainte de promovare.')}finally{setReuseBusy(false)}}
  return <article className={`rounded-xl border bg-[var(--surface)] p-4 ${decision.state === 'INVALID' || decision.state === 'UNRESOLVED' || decision.state === 'REJECTED' ? 'border-[var(--danger-border)]' : validProposal ? 'border-[var(--warning-border)]' : 'border-[var(--border)]'}`} aria-label={CLASSIFICATION_DIMENSION_LABELS[decision.dimension]}>
    <div className="flex items-start justify-between gap-2"><p className="eyebrow">{CLASSIFICATION_DIMENSION_LABELS[decision.dimension]}</p><DecisionBadge state={decision.state}/></div>
    <p className="mt-3 text-base font-bold">{decision.dimension === 'ACCOUNT' && value?.account ? <AccountValue code={value.account}/> : decision.dimension === 'EXPENSE_TAX_TREATMENT' && value?.kind === 'NOT_APPLICABLE' && invoice.accountingSnapshot?.profile?.taxRegime === 'MICROENTERPRISE' ? 'Nu se aplică pentru regimul fiscal curent' : displayDomainValue(value, fallback)}</p>
    <p className="mt-2 text-xs text-[var(--text-secondary)]">Sursă: {sourceLabel(record?.effectiveSource ?? record?.source)}</p>
    {record?.explanation && <details className="mt-3 text-xs"><summary className="cursor-pointer font-semibold text-[var(--accent)]">De ce a propus Diana această valoare?</summary><p className="mt-2 leading-5 text-[var(--text-secondary)]">{record.explanation}</p></details>}
    {/* Validation results describe the original AI proposal; once the decision is final they are audit history, not open actions. */}
    {canEdit && record?.validationResults?.map(issue => <ValidationIssue key={issue.code} issue={issue} item={item} invoice={invoice} onEdit={() => decision.dimension === 'ACCOUNT' ? setAccountEditing(true) : setEditing(true)}/>) }
    {!!record?.legalCitations?.length && <details className="mt-3 border-t border-[var(--border)] pt-3 text-xs"><summary className="cursor-pointer font-semibold"><Scale className="mr-1 inline size-3.5"/>Vezi baza legală</summary><div className="mt-2 space-y-2">{record.legalCitations.map(citation => <div key={`${citation.fragmentId}:${citation.citationKey}`} className={`rounded-lg p-2 ${citation.verified ? 'bg-[var(--surface-subtle)]' : 'border border-[var(--warning-border)] bg-[var(--warning-soft)]'}`}><strong>{citation.citationKey}</strong><p className="mt-1 text-[var(--text-muted)]">Versiune: {citation.versionId}</p>{!citation.verified && <p className="mt-1 font-semibold text-[var(--warning)]">Referința legală nu a putut fi verificată.</p>}</div>)}</div></details>}
    {record?.legalBasis && !record.legalCitations?.length && <details className="mt-3 border-t border-[var(--border)] pt-3 text-xs"><summary className="cursor-pointer font-semibold">Vezi baza legală</summary><p className="mt-2 text-[var(--text-secondary)]">{record.legalBasis}</p></details>}
    {record?.knowledge&&<p className="mt-3 rounded-lg bg-[var(--surface-subtle)] p-2 text-xs text-[var(--text-secondary)]">Bazată pe factura {record.knowledge.sourceInvoiceId}, aprobată de {record.knowledge.promotedBy} la {formatDateTime(record.knowledge.promotedAt)}.</p>}
    {canEdit && <div className="mt-4 flex flex-wrap gap-2 border-t border-[var(--border)] pt-3">
      {validProposal && <Button onClick={() => item && reviewMutation.mutate({ itemId:item.id, action:'APPROVE', reason:'Propunere verificată și aprobată.' })} disabled={reviewMutation.isPending}><Check className="size-4"/>Aprobă</Button>}
      <Button variant="secondary" onClick={() => decision.dimension === 'ACCOUNT' ? setAccountEditing(true) : setEditing(true)}><Edit3 className="size-4"/>Modifică</Button>
      {decision.state !== 'REJECTED' && <Button variant="ghost" onClick={() => setRejecting(true)}><X className="size-4"/>Respinge</Button>}
    </div>}
    {canReuse&&<div className="mt-3 border-t border-[var(--border)] pt-3"><Button variant="ghost" disabled={reuseBusy} onClick={openReuse}>{reuseBusy?'Se pregătește scope-ul…':'Folosește pentru situații similare'}</Button></div>}
    {reuseMessage&&<p role="status" className="mt-2 text-xs text-[var(--text-secondary)]">{reuseMessage}</p>}
    {reusePreview&&<ReuseConfirmation preview={reusePreview} busy={reuseBusy} onCancel={()=>setReusePreview(undefined)} onConfirm={confirmReuse}/>} 
    {reviewMutation.isError && <p role="alert" className="mt-2 text-xs text-[var(--danger)]">Decizia nu a putut fi salvată. Reîncarcă factura și încearcă din nou.</p>}
    {item && editing && <DomainCorrectionDialog item={item} invoice={invoice} onCancel={() => setEditing(false)} onSubmit={(typedValue, reason) => { reviewMutation.mutate({ itemId:item.id, typedValue, reason, action:'EDIT' }); setEditing(false) }}/>} 
    {item && accountEditing && <AccountDecisionDialog item={item} onCancel={() => setAccountEditing(false)} onSubmit={(account, mappingAction, reason) => { reviewMutation.mutate({ itemId:item.id, typedValue:{kind:'ACCOUNT',account:account.code},reason,mappingAction,expectedMappingRevision:item.mapping?.revision,action:'EDIT' });setAccountEditing(false) }}/>} 
    {item && rejecting && <RejectDialog reason={rejectReason} setReason={setRejectReason} onCancel={() => setRejecting(false)} onConfirm={() => { reviewMutation.mutate({itemId:item.id,reason:rejectReason,action:'REJECT'});setRejecting(false);setRejectReason('') }}/>} 
  </article>
}

function ReuseConfirmation({preview,busy,onCancel,onConfirm}:{preview:PromotionPreview;busy:boolean;onCancel:()=>void;onConfirm:()=>void}){return <div role="dialog" aria-modal="true" aria-labelledby={`reuse-${preview.classificationId}`} className="mt-3 rounded-xl border border-[var(--info-border)] bg-[var(--info-soft)] p-4 text-xs"><h4 id={`reuse-${preview.classificationId}`} className="text-sm font-bold">Confirmă reutilizarea deciziei</h4><p className="mt-3 font-semibold">Decizie: {CLASSIFICATION_DIMENSION_LABELS[preview.dimension]} = {displayDomainValue(preview.value,preview.value.account??preview.value.kind)}</p><dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1"><dt>Client</dt><dd>{preview.scope.clientDisplay}</dd><dt>Furnizor</dt><dd>{preview.scope.supplierDisplay}</dd><dt>Identitate serviciu</dt><dd>{identityLabel(preview.scope.serviceIdentityKind)} = {preview.scope.serviceIdentityValue}</dd><dt>Document</dt><dd>{preview.scope.documentType} · {preview.scope.currency} · TVA {preview.scope.vatRate}</dd><dt>Context</dt><dd>Profil contabil v{preview.scope.profileVersion}</dd></dl>{preview.scope.serviceIdentityKind==='NORMALIZED_DESCRIPTION'&&<p className="mt-3 font-semibold text-[var(--warning)]">Match-ul exact se bazează pe descrierea normalizată deoarece factura nu conține un ID de articol mai puternic.</p>}<div className="mt-4 flex gap-2"><Button variant="secondary" onClick={onCancel}>Anulează</Button><Button disabled={busy} onClick={onConfirm}>{busy?'Se salvează…':'Confirmă reutilizarea'}</Button></div></div>}

function ValidationIssue({ issue, item, invoice, onEdit }: { issue: NonNullable<LineClassification['validationResults']>[number]; item?: ClassificationReviewItem; invoice: Invoice; onEdit: () => void }) {
  const review = useReviewClassification(invoice.id)
  const message = validationMessage(issue.code, issue.message)
  return <div role="alert" className="mt-3 rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] p-3 text-xs"><p className="font-semibold text-[var(--danger)]">{message}</p>{issue.suggestedAccounts?.length ? <div className="mt-2"><p className="mb-2 text-[var(--text-secondary)]">Conturi analitice sugerate:</p><div className="flex flex-wrap gap-2">{issue.suggestedAccounts.map(code => <SuggestedAccount key={code} code={code} disabled={!item || review.isPending} onSelect={() => item && review.mutate({itemId:item.id,typedValue:{kind:'ACCOUNT',account:code},reason:`Contul analitic ${code} a fost selectat pentru această linie.`,mappingAction:'OCCURRENCE_ONLY',action:'EDIT'})}/>)}</div></div> : <Button className="mt-2" variant="secondary" onClick={onEdit}>Corectează valoarea</Button>}</div>
}

function SuggestedAccount({ code, disabled, onSelect }: { code: string; disabled: boolean; onSelect: () => void }) {
  const repository = useInvoiceRepository(); const [name, setName] = useState('')
  useEffect(() => { let active = true; void repository.searchAccounts(code).then(results => { if (active) setName(results.find(account => account.code === code)?.name ?? '') }); return () => { active = false } }, [code, repository])
  return <Button variant="secondary" disabled={disabled} onClick={onSelect} aria-label={`Selectează contul ${code}${name ? ` — ${name}` : ''}`}>{code}{name ? ` — ${name}` : ''}</Button>
}

function RejectDialog({ reason, setReason, onCancel, onConfirm }: { reason: string; setReason: (value:string)=>void; onCancel:()=>void; onConfirm:()=>void }) {
  return <div className="mt-3 rounded-lg border border-[var(--warning-border)] bg-[var(--warning-soft)] p-3"><label className="text-xs font-semibold">Motivul respingerii<textarea autoFocus value={reason} onChange={event => setReason(event.target.value)} className="mt-2 min-h-20 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] p-2"/></label><div className="mt-2 flex gap-2"><Button variant="ghost" onClick={onCancel}>Anulează</Button><Button variant="danger" disabled={!reason.trim()} onClick={onConfirm}>Respinge propunerea</Button></div></div>
}

function DecisionBadge({ state }: { state: DecisionState }) {
  const config = { FINAL:['success','Finală'], VALID_PROPOSAL:['warning','De verificat'], INVALID:['danger','Propunere invalidă'], UNRESOLVED:['danger','Nerezolvată'], REJECTED:['warning','Respinsă'] } as const
  return <Badge tone={config[state][0]}>{config[state][1]}</Badge>
}

function AccountValue({ code }: { code: string }) {
  const repository = useInvoiceRepository(); const [name, setName] = useState('')
  useEffect(() => { let active = true; void repository.searchAccounts(code).then(results => { if (active) setName(results.find(account => account.code === code)?.name ?? '') }); return () => { active = false } }, [code, repository])
  return <>{code}{name ? ` — ${name}` : ''}</>
}

function DerivedMonograph({ invoice }: { invoice: Invoice }) {
  const accounts = invoice.lines.flatMap(line => line.classifications.filter(item => item.dimension === 'ACCOUNT' && item.status !== 'PENDING' && item.typedValue?.account).map(item => ({line:line.description, account:item.typedValue!.account!})))
  return <details className="card"><summary className="cursor-pointer list-none p-5 font-bold outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]">Monografie contabilă <span className="ml-2 text-xs font-normal text-[var(--text-muted)]">derivată · doar pentru consultare</span></summary><div className="border-t border-[var(--border)] p-5 text-sm"><p className="text-[var(--text-secondary)]">Această vedere este derivată din deciziile finale de mai sus și nu poate fi editată independent.</p>{accounts.length ? <ul className="mt-3 space-y-2">{accounts.map((item,index)=><li key={`${item.account}:${index}`}><strong>{item.account}</strong> — {item.line}</li>)}</ul> : <p className="mt-3 text-[var(--text-muted)]">Monografia va fi disponibilă după finalizarea conturilor.</p>}</div></details>
}

function decisionFor(finals: LineClassification[], reviews: ClassificationReviewItem[], lineId: string, dimension: RuleCategory): Decision {
  const review = reviews.find(item => item.lineId === lineId && item.dimension === dimension)
  const final = finals.find(item => item.dimension === dimension)
  if (review?.status === 'REJECTED') return {dimension,review,final,state:'REJECTED'}
  if (review?.status === 'ACCEPTED' || review?.status === 'CORRECTED') return {dimension,review,final,state:'FINAL'}
  if (review?.validationResults?.length) return {dimension,review,final,state:'INVALID'}
  if (review?.status === 'PENDING' && review.proposedTypedValue) return {dimension,review,final,state:'VALID_PROPOSAL'}
  if (review) return {dimension,review,final,state:'UNRESOLVED'}
  if (final) return {dimension,final,state:'FINAL'}
  return {dimension,state:'UNRESOLVED'}
}

function countDecisions(decisions: Decision[]) { return decisions.reduce((result, decision) => { if (decision.state === 'FINAL') result.final++; else if (decision.state === 'VALID_PROPOSAL') result.valid++; else result.problem++; return result }, {final:0,valid:0,problem:0}) }
function SummaryBadge({ tone, value, label }: { tone:'success'|'warning'|'danger';value:number;label:string }) { return <Badge tone={tone}>{value} {label}</Badge> }
function sourceLabel(source?: string) { return ({RULE:'Regulă verificată',DETERMINISTIC_RULE:'Regulă verificată',LEARNED_MAPPING:'Decizie aprobată anterior',AI_PROPOSAL:'Propunere automată',PROFILE:'Profil fiscal aprobat',MANUAL:'Selectat manual',NO_MATCH:'Nicio regulă aplicabilă',AMBIGUOUS:'Mai multe variante posibile'} as Record<string,string>)[source ?? ''] ?? 'Sursă contabilă' }
function validationMessage(code: string, fallback: string) { return ({ACCOUNT_NOT_FOUND:'Contul propus nu există în planul de conturi.',ACCOUNT_INACTIVE:'Contul există, dar este inactiv.',ACCOUNT_NOT_POSTABLE:'Contul este sintetic și nu poate fi utilizat direct. Selectează un cont analitic.',ACCOUNT_NOT_ALLOWED_BY_PROFILE:'Contul nu este disponibil în configurația contabilă a acestui client.'} as Record<string,string>)[code] ?? fallback }
function staleReasons(reasons:string[]) { const labels:Record<string,string>={FISCAL_PROFILE_CHANGED:'Profilul fiscal s-a modificat.',ACCOUNT_CATALOG_CHANGED:'Planul de conturi s-a modificat.',ACCOUNTING_POLICY_CHANGED:'Politica contabilă s-a modificat.',CONTRACT_CONTEXT_CHANGED:'Contextul contractual s-a modificat.'};return reasons.length ? reasons.map(reason=>labels[reason]??'Contextul contabil s-a modificat.').join(' ') : 'Reanalizează factura pentru a folosi configurația curentă.' }
function profileLabel(value?:string){return ({PROFIT_TAX:'Impozit pe profit',MICROENTERPRISE:'Microîntreprindere'} as Record<string,string>)[value??'']??'Regim fiscal configurat'}
function formatMoney(amount:number,currency:string){return new Intl.NumberFormat('ro-RO',{style:'currency',currency}).format(amount)}
function formatDateTime(value:string){return new Intl.DateTimeFormat('ro-RO',{dateStyle:'medium',timeStyle:'short',timeZone:'Europe/Bucharest'}).format(new Date(value))}
function identityLabel(value:string){return ({SELLER_ITEM_ID:'ID articol furnizor',STANDARD_ITEM_ID:'ID standard',NORMALIZED_DESCRIPTION:'Descriere normalizată'} as Record<string,string>)[value]??value}
