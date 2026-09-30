import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { CommercialRule, ContractDocument, ProposedCommercialClause } from '../../repositories/invoiceRepository'
import { useActivateReviewedServicePrices, useCommercialServiceAliases, useConfirmProposedCommercialRule, useDismissProposedCommercialClause, useReviseConfirmedCommercialRule } from './contract-hooks'
import { buildConfirmedItems, clauseReasonLabel, coverageLabel, identityParties, invoiceCheckedKinds, skippedReasonLabel, type ReviewItem } from './contract-review-model'
import { ExtractionHistory, RuleExplanation, RuleFormula } from './contract-review-parts'
import { ContractReviewWorkspace } from './ContractReviewWorkspace'
import { RevokeAliasButton } from './RevokeAliasButton'
import { useContractPdf } from './useContractPdf'

type Activation = ReturnType<typeof useActivateReviewedServicePrices>

// A confirmed contract document: every value next to the PDF words it came
// from, what each tariff and clause does for invoice checks now, and the
// clauses still to settle so the contract's coverage can become complete.
export function ConfirmedContractWorkspace({ document }: { document: ContractDocument }) {
  const pdf = useContractPdf(document.clientId, document.id)
  const items = useMemo(() => buildConfirmedItems(document), [document])
  const activation = useActivateReviewedServicePrices(document.clientId, document.id)
  const coverage = coverageLabel(document.commercialState)
  const confirmed = document.confirmedValues
  const confirmedAt = document.confirmedAt ? new Date(document.confirmedAt).toLocaleDateString('ro-RO') : ''
  return (
    <ContractReviewWorkspace
      document={document}
      items={items}
      pdf={pdf}
      eyebrow="Contract confirmat"
      title={document.originalFilename}
      subtitle={<>{document.clientName ?? document.clientId}{confirmed?.reference ? ` · ${confirmed.reference}` : ''} · confirmat de {document.confirmedBy ?? 'utilizator'}{confirmedAt ? ` la ${confirmedAt}` : ''}</>}
      badges={<><Badge tone={coverage.tone}>{coverage.text}</Badge>{document.commercialState && <Badge tone="neutral">Versiunea {document.commercialState.snapshotVersion}</Badge>}</>}
      actions={document.confirmedContractId && <Link className="inline-flex h-10 items-center rounded-lg border border-[var(--border-strong)] px-4 text-sm font-semibold hover:bg-[var(--surface-subtle)]" to={`/contracts/${document.confirmedContractId}`}>Deschide contractul autoritativ</Link>}
      nextLabel={(count) => count === 1 ? '1 de rezolvat' : `${count} de rezolvat`}
      renderDetail={(item) => <ConfirmedItemDetail document={document} item={item} activation={activation} />}
      afterList={<LearnedWordings document={document} />}
      footer={<footer className="card px-5 py-3"><p className="text-xs break-all text-[var(--text-muted)]">SHA-256: {document.sha256}</p><ExtractionHistory document={document} /></footer>}
    />
  )
}

function ConfirmedItemDetail({ document, item, activation }: { document: ContractDocument; item: ReviewItem; activation: Activation }) {
  const kind = item.kind
  if (kind.type === 'service') {
    const inactive = (document.commercialState?.services ?? []).filter((service) => !service.active && !service.skipReason).length
    if (!kind.activatable && !activation.isSuccess) return null
    return <ActivatePrices activation={activation} inactive={inactive} />
  }
  if (kind.type !== 'clause') return null
  if (kind.status === 'ACTIVE' && kind.rule) return <ConfirmedRuleDetail document={document} rule={kind.rule} />
  if (kind.status === 'CLOSED') {
    const settled = document.commercialState?.clauses.find((clause) => clause.ruleId === kind.ruleId)
    return <p className="text-xs text-[var(--text-secondary)]">{clauseReasonLabel[settled?.reasonCode ?? ''] ?? 'Închisă'}{settled?.reviewedBy ? ` · ${settled.reviewedBy}` : ''}{settled?.reviewedAt ? `, ${new Date(settled.reviewedAt).toLocaleDateString('ro-RO')}` : ''}</p>
  }
  if (!kind.clause) return null
  return <PendingClause key={kind.ruleId} document={document} clause={kind.clause} kind={kind.kind} />
}

function ActivatePrices({ activation, inactive }: { activation: Activation; inactive: number }) {
  return (
    <div className="rounded-lg border border-[var(--warning-border)] bg-[var(--surface)] p-3">
      {!activation.isSuccess && <p className="text-sm">{inactive === 1 ? 'Un tarif confirmat nu este încă folosit' : `${inactive} tarife confirmate nu sunt încă folosite`} la verificarea facturilor. Se activează numai tarifele care au aceeași valoare și monedă în fragmentul citat din PDF.</p>}
      <Button type="button" variant="secondary" className="mt-2" disabled={activation.isPending || activation.isSuccess} onClick={() => activation.mutate()}>{activation.isPending ? 'Se activează…' : 'Activează tarifele confirmate'}</Button>
      {activation.isSuccess && <p className="mt-2 text-sm text-[var(--success)]">{activation.data.activated === 1 ? 'Un tarif activat' : `${activation.data.activated} tarife activate`}. Revino la factură și rulează validarea din nou.</p>}
      {activation.isSuccess && !!activation.data.skipped?.length && <div role="status" className="mt-2 text-sm"><p className="font-semibold text-[var(--warning)]">Nu au fost activate:</p><ul className="mt-1 list-disc space-y-1 pl-5">{activation.data.skipped.map((skipped) => <li key={skipped.position}>{skipped.description || `Serviciul ${skipped.position}`}{skipped.unitPrice ? ` · ${skipped.unitPrice} ${skipped.currency ?? ''}` : ''}: {skippedReasonLabel[skipped.reason] ?? skipped.reason}</li>)}</ul></div>}
      {activation.isError && <p role="alert" className="mt-2 text-sm text-[var(--danger)]">Tarifele nu au putut fi activate. Verifică dacă prețul și moneda sunt susținute de PDF.</p>}
    </div>
  )
}

function ConfirmedRuleDetail({ document, rule }: { document: ContractDocument; rule: CommercialRule }) {
  const [editing, setEditing] = useState(false)
  const proposal = document.extraction?.proposal
  const editable = ['VAT', 'PAYMENT_DUE'].includes(rule.kind) && (proposal?.commercialClauses ?? []).some((clause) => clause.rule?.id === rule.id && !clause.rule?.expression)
  return (
    <div>
      {rule.origin === 'SOURCE_TEXT' && <p className="text-xs text-[var(--text-muted)]">Recunoscută automat din textul clauzei</p>}
      <RuleFormula rule={rule} />
      {editable && (editing ? <RuleValuesEditor document={document} rule={rule} onDone={() => setEditing(false)} /> : <Button type="button" variant="secondary" className="mt-2" onClick={() => setEditing(true)}>Modifică valorile</Button>)}
    </div>
  )
}

// A clause still proposed: complete its value, confirm the proposed rule, or
// close it — covered by the parties' CUIs, or without effect on invoices.
function PendingClause({ document, clause, kind }: { document: ContractDocument; clause: ProposedCommercialClause; kind: string }) {
  const confirmRule = useConfirmProposedCommercialRule(document.clientId, document.id)
  const dismiss = useDismissProposedCommercialClause(document.clientId, document.id)
  const supplierCui = document.confirmedValues?.supplierCui
  const buyerCui = document.confirmedValues?.buyerCui
  const ruleId = clause.rule.id
  const parties = kind === 'IDENTITY' ? identityParties(clause.evidence.snippet, supplierCui, buyerCui) : undefined
  // Invoices are compared with an identity rule by their supplier CUI only.
  const foreignIdentity = kind === 'IDENTITY' && Boolean(clause.rule.expression) && digitsOf(clause.rule.expression?.value) !== digitsOf(supplierCui)
  return (
    <div className="space-y-3">
      {parties ? (
        <div className="rounded-lg border border-[var(--border)] bg-[var(--surface)] p-3">
          <p className="text-xs">{partiesExplanation[parties]}</p>
          <Button type="button" className="mt-2" disabled={dismiss.isPending} onClick={() => dismiss.mutate({ ruleId, reasonCode: parties === 'SUPPLIER' ? 'COVERED_BY_SUPPLIER_IDENTITY' : 'COVERED_BY_PARTY_IDENTITY' })}>{dismiss.isPending ? 'Se închide…' : parties === 'SUPPLIER' ? `Închide: acoperită de CUI-ul ${supplierCui}` : parties === 'BUYER' ? `Închide: acoperită de CUI-ul clientului ${buyerCui}` : `Închide: acoperită de CUI-urile părților (${supplierCui}, ${buyerCui})`}</Button>
        </div>
      ) : kind === 'CONTRACT_REFERENCE' ? (
        <p className="text-xs text-[var(--text-muted)]">Referința contractului este deja acoperită de regula de identificare confirmată.</p>
      ) : foreignIdentity ? (
        <p className="text-xs text-[var(--warning)]">Clauza numește un alt CUI decât al furnizorului ({supplierCui}). Pe factură Diana compară doar furnizorul, așa că nu poate deveni regulă. Verifică în PDF cine este această parte.</p>
      ) : clause.rule.expression ? (
        <div><RuleExplanation rule={clause.rule} /><Button type="button" variant="secondary" className="mt-2" disabled={confirmRule.isPending} onClick={() => confirmRule.mutate({ ruleId })}>{confirmRule.isPending ? 'Se confirmă…' : 'Confirmă regula'}</Button></div>
      ) : (
        <IncompleteRuleReview document={document} clause={clause} />
      )}
      {!parties && <DismissClause kind={kind} dismiss={dismiss} ruleId={ruleId} />}
      {confirmRule.isError && <p role="alert" className="text-xs text-[var(--danger)]">Regula nu a putut fi confirmată.</p>}
      {dismiss.isError && <p role="alert" className="text-xs text-[var(--danger)]">Clauza nu a putut fi închisă.</p>}
    </div>
  )
}

const digitsOf = (value: string | undefined) => (value ?? '').replace(/\D/g, '')

const partiesExplanation = {
  SUPPLIER: 'Clauza doar identifică furnizorul prin CUI-ul confirmat al contractului. Facturile se asociază contractului după acest CUI, deci nu mai e nimic de verificat aici.',
  BUYER: 'Clauza doar identifică clientul (beneficiarul) prin CUI-ul lui. Diana primește din SPV numai facturile emise către acest CUI, deci nu mai e nimic de verificat aici.',
  BOTH: 'Clauza doar identifică părțile contractului prin CUI-urile lor. Furnizorul leagă factura de contract, iar clientul este cumpărătorul fiecărei facturi primite din SPV, deci nu mai e nimic de verificat aici.',
} as const

// Every confirmed clause becomes a check run on each invoice of the contract,
// and a clause left to settle keeps them all in commercial review. A clause
// without effect on invoices is closed here, with the reviewer's reason.
function DismissClause({ kind, ruleId, dismiss }: { kind: string; ruleId: string; dismiss: ReturnType<typeof useDismissProposedCommercialClause> }) {
  const [open, setOpen] = useState(false)
  const [reason, setReason] = useState('')
  const valid = reason.trim().length >= 10
  if (!open) return <button type="button" className="text-xs font-semibold text-[var(--accent)] underline" onClick={() => setOpen(true)}>Închide clauza: nu influențează facturile…</button>
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--surface)] p-3">
      <p className="text-xs text-[var(--text-secondary)]">Fiecare clauză confirmată devine o verificare pe fiecare factură din acest contract. Cât timp o clauză este „De rezolvat”, toate facturile contractului rămân în verificare comercială. Închide-o doar dacă nu privește sume, cote TVA sau termene de plată.</p>
      <label className="mt-2 block text-xs font-semibold" htmlFor={`dismiss-${ruleId}`}>De ce nu influențează facturile?</label>
      <textarea id={`dismiss-${ruleId}`} value={reason} onChange={(event) => setReason(event.target.value)} maxLength={500} rows={2} placeholder="Ex.: clauză de confidențialitate, fără efect asupra facturii" className="mt-1 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 py-2 text-sm" />
      {invoiceCheckedKinds.has(kind) && <p className="mt-1 text-xs text-[var(--warning)]">Atenție: termenii acestei clauze nu vor mai fi verificați pe facturi.</p>}
      <div className="mt-2 flex gap-2">
        <Button type="button" variant="secondary" disabled={!valid || dismiss.isPending} onClick={() => dismiss.mutate({ ruleId, reasonCode: 'NOT_INVOICE_VERIFIABLE', reason: reason.trim() })}>{dismiss.isPending ? 'Se închide…' : 'Închide clauza'}</Button>
        <Button type="button" variant="secondary" onClick={() => { setOpen(false); setReason('') }}>Renunță</Button>
      </div>
      {!valid && <p className="mt-1 text-xs text-[var(--text-muted)]">Motivul are cel puțin 10 caractere și rămâne în istoricul contractului.</p>}
    </div>
  )
}

function IncompleteRuleReview({document,clause}:{document:ContractDocument;clause:ProposedCommercialClause}){
  const confirm=useConfirmProposedCommercialRule(document.clientId,document.id)
  const [value,setValue]=useState(()=>clause.rule?.kind==='PAYMENT_DUE'?singleStatedValue(`${clause.narrative.value??''} ${clause.evidence.snippet??''}`,/\b(\d+)\s*(?:\([^)]{1,40}\)\s*)?(?:de\s+)?zile\b/gi):'')
  const [currency,setCurrency]=useState(document.confirmedValues?.currency??'RON')
  const kind=clause.rule?.kind
  const clauseText=`${clause.narrative.value??''} ${clause.evidence.snippet??''}`
  const [dateBasis,setDateBasis]=useState(()=>inferredDateBasis(clauseText))
  if(kind==='VAT'&&(!/\d+(?:[.,]\d+)?\s*%/.test(clauseText)||legalVATPattern.test(clauseText))){
    const confirmApplicableVAT=()=>{if(!clause.rule)return;const rule:CommercialRule={...clause.rule,dateBasis:'INVOICE_ISSUE_DATE',currency:undefined,expression:{op:'variable',variable:'applicable_vat_rate'},requiredVariables:['applicable_vat_rate'],evidence:clause.rule.evidence??[],blocking:true};confirm.mutate({ruleId:rule.id,rule})}
    return <div className="rounded border border-[var(--border)] bg-[var(--surface)] p-3"><p className="text-xs">Contractul nu fixează un procent; cere aplicarea cotei TVA valabile. Confirmă tratamentul contractual, iar cota și sursa fiscală se vor completa separat, cu perioada lor de valabilitate.</p><Button type="button" variant="secondary" className="mt-2" disabled={confirm.isPending||confirm.isSuccess} onClick={confirmApplicableVAT}>{confirm.isSuccess?'Tratament TVA confirmat':'Confirmă „TVA aplicabilă”'}</Button>{confirm.isError&&<p role="alert" className="mt-2 text-xs text-[var(--danger)]">Tratamentul TVA nu a putut fi confirmat.</p>}</div>
  }
  if(!['FIXED_PRICE','UNIT_RATE','VAT','PAYMENT_DUE'].includes(kind??''))return <p className="text-xs text-[var(--warning)]">Diana nu are o verificare automată pentru acest tip de clauză, așa că o ține deschisă până decizi. Dacă nu privește sume, cote TVA sau termene de plată, închide-o mai jos, cu motiv.</p>
  const valid=/^-?\d+(?:\.\d+)?$/.test(value)&&(!['FIXED_PRICE','UNIT_RATE'].includes(kind!)||/^[A-Z]{3}$/.test(currency))
  const submit=()=>{
    if(!valid||!clause.rule)return
    const rule:CommercialRule={...clause.rule,dateBasis:kind==='PAYMENT_DUE'?dateBasis:(clause.rule.dateBasis||'INVOICE_ISSUE_DATE'),currency:['FIXED_PRICE','UNIT_RATE'].includes(kind!)?currency:undefined,expression:{op:'literal',value,scale:2},evidence:clause.rule.evidence??[],blocking:true}
    confirm.mutate({ruleId:rule.id,rule})
  }
  const label=kind==='PAYMENT_DUE'?'Număr de zile':kind==='VAT'?'Cotă TVA (%)':'Valoare contractuală'
  return <div className="rounded border border-[var(--border)] bg-[var(--surface)] p-3"><p className="text-xs">Completează numai valoarea explicită din clauza evidențiată în PDF.</p><div className="mt-2 grid gap-2 sm:grid-cols-2"><label className="text-xs">{label}<input aria-label={`${label} ${clause.rule.id}`} value={value} onChange={event=>setValue(event.target.value)} className="mt-1 w-full rounded border px-2 py-1"/></label>{['FIXED_PRICE','UNIT_RATE'].includes(kind!)&&<label className="text-xs">Monedă<input aria-label={`Monedă ${clause.rule.id}`} value={currency} onChange={event=>setCurrency(event.target.value.toUpperCase())} className="mt-1 w-full rounded border px-2 py-1"/></label>}{kind==='PAYMENT_DUE'&&<label className="text-xs">Calculat de la<select aria-label={`Bază termen ${clause.rule.id}`} value={dateBasis} onChange={event=>setDateBasis(event.target.value)} className="mt-1 w-full rounded border px-2 py-1"><option value="RECEIPT_DATE">Primirea/remiterea facturii</option><option value="INVOICE_ISSUE_DATE">Data emiterii</option><option value="ACCEPTANCE_DATE">Acceptarea serviciului</option></select></label>}</div><Button type="button" variant="secondary" className="mt-2" disabled={!valid||confirm.isPending||confirm.isSuccess} onClick={submit}>{confirm.isSuccess?'Regulă confirmată':'Confirmă regula completată'}</Button>{confirm.isError&&<p role="alert" className="mt-2 text-xs text-[var(--danger)]">Regula completată nu a putut fi confirmată.</p>}</div>
}
// Pre-completări pentru formularul manual; serverul verifică oricum că valoarea apare în clauză.
const legalVATPattern=/legal|aplicabil|aferent/i
function inferredDateBasis(text:string){const lower=text.toLowerCase();const bases=[/\bemiter/.test(lower)&&'INVOICE_ISSUE_DATE',/remiter|primir|transmiter/.test(lower)&&'RECEIPT_DATE',/accept/.test(lower)&&'ACCEPTANCE_DATE'].filter(Boolean) as string[];return bases.length===1?bases[0]:'RECEIPT_DATE'}
function singleStatedValue(text:string,pattern:RegExp){const values=new Set([...text.matchAll(pattern)].map(match=>match[1]));return values.size===1?[...values][0]:''}

function RuleValuesEditor({document,rule,onDone}:{document:ContractDocument;rule:CommercialRule;onDone:()=>void}){
  const revise=useReviseConfirmedCommercialRule(document.clientId,document.id)
  const applicable=rule.expression?.op==='variable'&&rule.expression.variable==='applicable_vat_rate'
  const [vatMode,setVatMode]=useState<'LEGAL'|'FIXED'>(applicable?'LEGAL':'FIXED')
  const [value,setValue]=useState(rule.expression?.op==='literal'?rule.expression.value??'':'')
  const [dateBasis,setDateBasis]=useState(rule.dateBasis||'INVOICE_ISSUE_DATE')
  const legal=rule.kind==='VAT'&&vatMode==='LEGAL'
  const valid=legal||/^\d+(?:\.\d+)?$/.test(value)
  const submit=()=>{
    if(!valid)return
    const next:CommercialRule=legal?{...rule,dateBasis:'INVOICE_ISSUE_DATE',currency:undefined,expression:{op:'variable',variable:'applicable_vat_rate'},requiredVariables:['applicable_vat_rate'],blocking:true}:{...rule,dateBasis:rule.kind==='PAYMENT_DUE'?dateBasis:'INVOICE_ISSUE_DATE',currency:undefined,expression:{op:'literal',value,scale:2},requiredVariables:undefined,blocking:true}
    revise.mutate({ruleId:rule.id,rule:next},{onSuccess:onDone})
  }
  return <div className="mt-3 rounded border border-[var(--border)] bg-[var(--surface)] p-3"><p className="text-xs">Modificarea creează o versiune nouă. Valoarea trebuie să apară în clauza citată.</p>{rule.kind==='VAT'&&<fieldset className="mt-2 flex flex-wrap gap-4 text-xs"><label><input type="radio" checked={vatMode==='LEGAL'} onChange={()=>setVatMode('LEGAL')}/> Cota legală valabilă la data facturii</label><label><input type="radio" checked={vatMode==='FIXED'} onChange={()=>setVatMode('FIXED')}/> Cotă fixă din contract</label></fieldset>}<div className="mt-2 grid gap-2 sm:grid-cols-2">{!legal&&<label className="text-xs">{rule.kind==='PAYMENT_DUE'?'Număr de zile':'Cotă TVA (%)'}<input aria-label={`Valoare modificată ${rule.id}`} value={value} onChange={event=>setValue(event.target.value)} className="mt-1 w-full rounded border px-2 py-1"/></label>}{rule.kind==='PAYMENT_DUE'&&<label className="text-xs">Calculat de la<select aria-label={`Bază termen modificată ${rule.id}`} value={dateBasis} onChange={event=>setDateBasis(event.target.value)} className="mt-1 w-full rounded border px-2 py-1"><option value="RECEIPT_DATE">Primirea/remiterea facturii</option><option value="INVOICE_ISSUE_DATE">Data emiterii</option><option value="ACCEPTANCE_DATE">Acceptarea serviciului</option></select></label>}</div><div className="mt-2 flex gap-2"><Button type="button" variant="secondary" disabled={!valid||revise.isPending} onClick={submit}>{revise.isPending?'Se salvează…':'Salvează modificarea'}</Button><Button type="button" variant="secondary" onClick={onDone}>Renunță</Button></div>{revise.isError&&<p role="alert" className="mt-2 text-xs text-[var(--danger)]">Modificarea nu a putut fi salvată. Verifică dacă valoarea și baza de calcul apar în clauza contractului.</p>}</div>
}

// Invoice wordings the reviewers taught Diana for this contract, so each can
// be checked and revoked where the contract itself is maintained.
function LearnedWordings({document}:{document:ContractDocument}){
  const aliases=useCommercialServiceAliases(document.clientId,{documentId:document.id})
  const learned=(aliases.data??[]).filter(alias=>!alias.invoiceId)
  if(!learned.length)return null
  return <section aria-label="Formulări învățate de pe facturi" className="border-t border-[var(--border)]"><h3 className="eyebrow bg-[var(--surface-subtle)] px-4 py-2">Formulări învățate de pe facturi ({learned.length})</h3><p className="px-4 pt-2 text-xs text-[var(--text-muted)]">Aceste formulări de pe facturile furnizorului sunt asociate automat cu serviciul din contract. Revocă o asociere dacă formularea nu înseamnă întotdeauna același serviciu.</p><ul className="divide-y divide-[var(--border)] text-sm">{learned.map(alias=><li key={alias.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-2.5"><div className="min-w-0"><p><span className="font-semibold">„{alias.normalizedLabel}”</span> → {alias.serviceLabel??alias.serviceId}</p><p className="text-xs text-[var(--text-muted)]">Confirmată de {alias.confirmedBy} · {new Date(alias.confirmedAt).toLocaleDateString('ro-RO')}{alias.revokedAt?` · revocată de ${alias.revokedBy||'utilizator'} la ${new Date(alias.revokedAt).toLocaleDateString('ro-RO')}`:''}</p></div>{alias.revokedAt?<Badge tone="neutral">Revocată</Badge>:<RevokeAliasButton clientId={document.clientId} alias={alias} appearance="button"/>}</li>)}</ul></section>
}
