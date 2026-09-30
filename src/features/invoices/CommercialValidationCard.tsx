import { ArrowRight, CircleCheck, CircleX, Link2, TriangleAlert, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { Invoice, InvoiceLine } from '../../domain/invoice'
import type { CommercialOutcome, CommercialValidationRun, LearnedServiceAlias } from '../../repositories/invoiceRepository'
import { useCommercialServiceAliases } from '../contracts/contract-hooks'
import { RevokeAliasButton } from '../contracts/RevokeAliasButton'
import { buildCommercialView, unitLabel, type CommercialCheck, type CommercialView } from './commercial-view'
import { CommercialEvidenceDialog, type EvidenceDialogState } from './CommercialEvidenceDialog'
import { useCommercialValidation, usePutCommercialDateFact, usePutCommercialVariable, useResolveCommercialValidation } from './invoice-hooks'

const outcomeLabel:Record<CommercialOutcome,string>={CONFORM:'Conform',NECONFORM:'Neconform',NEVERIFICABIL:'Nu poate fi verificat'}
const missingInputLabel=(name:string)=>({remittance_date:'data remiterii facturii',receipt_date:'data primirii facturii',acceptance_date:'data acceptării serviciului',invoice_due_date:'scadența facturii'}[name]??(name.startsWith('unit_quantity_')?'cantitatea serviciului':name))
const rowColumns='gap-x-3 px-4 sm:grid-cols-[20px_minmax(0,1.3fr)_minmax(0,1fr)_minmax(0,1fr)_128px] sm:items-center'
const rowGrid=`grid gap-y-1 py-2.5 ${rowColumns}`
const normalizeLabel=(value:string)=>value.trim().split(/\s+/).join(' ').toLocaleUpperCase('ro-RO')

export function CommercialValidationCard({invoice}:{invoice:Invoice}){
  const {data:run,isLoading}=useCommercialValidation(invoice)
  const resolve=useResolveCommercialValidation(invoice)
  const [searchParams,setSearchParams]=useSearchParams()
  const [mapped,setMapped]=useState(0)
  const view=useMemo(()=>run?buildCommercialView(run,invoice):undefined,[run,invoice])
  const aliases=useCommercialServiceAliases(invoice.clientId,run?.dossierId?{dossierId:run.dossierId}:undefined)
  if(['DOWNLOADED','ARCHIVED','MATCHING','AWAITING_CONTRACT','AWAITING_MATCH_CONFIRM','DEDUPE_CHECKED','HEADER_READ','LINES_READ','DUPLICATE'].includes(invoice.pipelineStatus))return null
  if(isLoading)return <section className="card p-5" role="status">Se încarcă validarea comercială…</section>
  if(!run||!view)return <section className="card p-5"><h3 className="font-bold">Validare comercială</h3><p className="mt-2 text-sm text-[var(--text-secondary)]">{mapped?'Asocierile au fost salvate. Validarea rulează din nou…':'Rezultatul nu este disponibil încă.'}</p></section>
  const reviewable=invoice.pipelineStatus==='AWAITING_COMMERCIAL_REVIEW'
  const dialog=dialogState(searchParams,view)
  const setDialog=(next:EvidenceDialogState|undefined)=>{const params=new URLSearchParams(searchParams);params.delete('verifica');params.delete('asociere');if(next?.mode==='verify')params.set('verifica',next.checkId);if(next?.mode==='map')params.set('asociere',next.lineId??'toate');setSearchParams(params,{replace:true})}
  // The coverage finding cites the open clauses; its document comes first.
  const coverageFinding=run.findings.find(finding=>finding.code==='CONTRACT_COVERAGE_INCOMPLETE')
  const documentId=[...(coverageFinding?.evidence??[]),...run.findings.flatMap(finding=>finding.evidence??[])].find(evidence=>evidence.documentId)?.documentId
  const needsContractReview=!view.mappings.length&&run.findings.some(finding=>finding.code==='CONTRACT_COVERAGE_INCOMPLETE'||(finding.code==='SERVICE_LINE_UNCOVERED'&&!finding.serviceCandidates?.length))
  const suggested=view.mappings.filter(({finding})=>finding.serviceCandidates?.some(candidate=>candidate.suggested)).length
  const act=(action:'RERUN')=>resolve.mutate({runId:run.id,expectedInvoiceRevision:invoice.revision??run.invoiceRevision+1,action})
  // The contract or its learned wordings changed after this run: the result
  // below is stale until the validation runs again.
  const contractChanged=Boolean(run.activeSnapshotVersion&&run.snapshotVersion&&run.activeSnapshotVersion!==run.snapshotVersion)
  const runAt=Date.parse(run.createdAt)
  const aliasesChanged=(aliases.data??[]).some(alias=>(!alias.invoiceId||alias.invoiceId===invoice.id)&&(Date.parse(alias.confirmedAt)>runAt||(alias.revokedAt?Date.parse(alias.revokedAt)>runAt:false)))
  const rowProps={invoice,run,reviewable,documentId,onVerify:(check:CommercialCheck)=>setDialog({mode:'verify',checkId:check.id}),onMap:(lineId?:string)=>setDialog({mode:'map',lineId})}
  return <section className="card p-5" aria-labelledby="commercial-title">
    <div className="flex items-start justify-between gap-4"><div><p className="eyebrow">Contract Intelligence</p><h3 id="commercial-title" className="mt-1 font-bold">Validare comercială</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">{view.checkCount} verificări{view.verifiable.length?' · apasă „Verifică” pentru a vedea factura și contractul față în față':''}</p></div>{view.pendingCount?<Badge tone={run.outcome==='NECONFORM'?'danger':'warning'}>{view.pendingCount} de rezolvat</Badge>:<OutcomeBadge outcome={run.outcome}/>}</div>
    {run.engineVersion==='COMMERCIAL_VALIDATION_V1'&&<p className="mt-3 text-xs text-[var(--text-muted)]">Rezultat calculat cu o versiune anterioară a motorului; cotele TVA sunt afișate ca verificări de TVA.{reviewable?' Rulează din nou validarea pentru rezultatul actual.':''}</p>}
    {reviewable&&!mapped&&(contractChanged||aliasesChanged)&&<div role="status" className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-[var(--info-border)] bg-[var(--info-soft)] px-4 py-3"><p className="text-sm">{contractChanged?`Contractul are acum versiunea ${run.activeSnapshotVersion}; rezultatul de mai jos a folosit versiunea ${run.snapshotVersion}.`:'Asocierile de servicii ale contractului s-au schimbat după această validare.'} Rulează din nou validarea ca să vezi rezultatul actual.</p><Button type="button" disabled={resolve.isPending} onClick={()=>act('RERUN')}>Rulează din nou</Button></div>}
    {mapped>0&&<div role="status" className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-[var(--success-border)] bg-[var(--success-soft)] px-4 py-2.5"><span className="flex items-center gap-2 text-sm"><CircleCheck aria-hidden="true" className="size-5 text-[var(--success)]"/>{mapped===1?'Asocierea a fost salvată':'Asocierile au fost salvate'}; validarea rulează din nou cu serviciile alese.</span><span className="flex items-center gap-2"><Link to="/tasks?type=COMMERCIAL_REVIEW" className="inline-flex h-9 items-center gap-1.5 rounded-lg bg-[var(--accent)] px-3 text-xs font-semibold text-[var(--accent-contrast)]">Următoarea factură de verificat<ArrowRight aria-hidden="true" className="size-4"/></Link><Button type="button" variant="ghost" size="icon" aria-label="Închide mesajul" onClick={()=>setMapped(0)}><X className="size-4"/></Button></span></div>}
    {reviewable&&view.mappings.length>0&&<div className="mt-4 flex flex-wrap items-center justify-between gap-4 rounded-xl border border-[var(--warning-border)] bg-[var(--warning-soft)] px-4 py-3"><div className="flex items-start gap-3"><TriangleAlert aria-hidden="true" className="mt-0.5 size-5 shrink-0 text-[var(--warning)]"/><div><p className="text-sm font-semibold">{view.mappings.length===1?'O linie nu este asociată':`${view.mappings.length} linii nu sunt asociate`} cu un serviciu din contract</p><p className="mt-0.5 text-sm text-[var(--text-secondary)]">{suggested?`Diana a pregătit ${suggested===view.mappings.length?(suggested===1?'o sugestie':'câte o sugestie'):`sugestii pentru ${suggested} dintre ele`}. Le confirmi într-un singur pas, iar validarea rulează din nou automat.`:'Alege serviciul din contract pentru fiecare linie; validarea rulează apoi automat.'}</p></div></div><Button type="button" onClick={()=>setDialog({mode:'map'})}>Asociază serviciile ({view.mappings.length})</Button></div>}
    {reviewable&&needsContractReview&&<div className="mt-4 rounded-lg border border-[var(--warning)] bg-[var(--surface)] p-4"><h4 className="text-sm font-bold">Ce ai de făcut acum</h4><ol className="mt-2 list-inside list-decimal space-y-2 text-sm"><li>Deschide contractul: rândurile marcate „De rezolvat” arată ce lipsește. Activează tarifele nefolosite, completează clauzele doar cu valori scrise în PDF sau închide-le cu motiv dacă nu influențează facturile; nu prelua sume sau cote TVA din factură. {documentId?<Link className="font-semibold text-[var(--accent)] underline" to={`/contracts/documents/${encodeURIComponent(invoice.clientId)}/${encodeURIComponent(documentId)}?element=group:clauses`}>Deschide contractul la ce e de rezolvat</Link>:<span>Deschide documentul contractual din pagina Contracte.</span>}</li><li>Revino la această factură și apasă „Rulează din nou după completări”.</li><li>Dacă apare apoi solicitarea de asociere a serviciilor, confirmă ce înseamnă formulările de pe factură pentru acest contract.</li></ol><p className="mt-2 text-xs text-[var(--text-secondary)]">Nu introduce motiv de excepție pentru informații încă neconfirmate.</p></div>}
    <div className="mt-4 overflow-hidden rounded-xl border border-[var(--border)]">
      <div className={`hidden py-2 sm:grid ${rowColumns} bg-[var(--surface-subtle)] text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]`}><span/><span>Verificare</span><span>Pe factură</span><span>În contract</span><span/></div>
      {view.invoiceChecks.length>0&&<div className="border-t border-[var(--border)] first:border-t-0"><GroupHeader label="Factură" detail="verificări pentru întreaga factură"/>{view.invoiceChecks.map(check=><CheckRow key={check.id} check={check} {...rowProps}/>)}</div>}
      {view.lineGroups.map(group=><div key={group.line.id} className="border-t border-[var(--border)]"><GroupHeader label={`Linia ${group.line.position}`} title={group.line.description} detail={`${unitLabel(group.line.unit)} · cant. ${group.line.quantity}`}/><LineAssociation line={group.line} serviceLabel={group.serviceLabel} aliases={aliases.data} invoice={invoice}/>{group.checks.map(check=><CheckRow key={check.id} check={check} {...rowProps}/>)}</div>)}
    </div>
    <div className="mt-3 flex flex-wrap items-center justify-between gap-3"><details className="text-xs text-[var(--text-muted)]"><summary>Detalii tehnice</summary>Motor {run.engineVersion} · snapshot v{run.snapshotVersion || 'legacy parțial'}</details>{reviewable&&<Button type="button" variant="secondary" disabled={resolve.isPending} onClick={()=>act('RERUN')}>Rulează din nou după completări</Button>}</div>
    {dialog&&<CommercialEvidenceDialog invoice={invoice} run={run} view={view} state={dialog} reviewable={reviewable} onStateChange={setDialog} onClose={()=>setDialog(undefined)} onMapped={count=>{setMapped(count);setDialog(undefined)}}/>}
  </section>
}

function dialogState(params:URLSearchParams,view:CommercialView):EvidenceDialogState|undefined{
  const checkId=params.get('verifica')
  if(checkId&&view.verifiable.some(check=>check.id===checkId||check.findings.some(finding=>finding.id===checkId)))return {mode:'verify',checkId:view.verifiable.find(check=>check.id===checkId||check.findings.some(finding=>finding.id===checkId))!.id}
  const lineId=params.get('asociere')
  if(lineId&&view.mappings.length)return {mode:'map',lineId:lineId==='toate'?undefined:lineId}
  return undefined
}

function GroupHeader({label,title,detail}:{label:string;title?:string;detail?:string}){return <div className="flex flex-wrap items-baseline gap-x-2.5 px-4 pb-1 pt-3"><span className="eyebrow">{label}</span>{title&&<span className="text-sm font-semibold">{title}</span>}{detail&&<span className="text-xs text-[var(--text-muted)]">{detail}</span>}</div>}

// Which contractual service a line was checked against and whether that
// association was learned for the whole contract (and can be revoked). A
// mapping confirmed on this invoice stays with it after the learned wording
// is revoked.
function LineAssociation({line,serviceLabel,aliases,invoice}:{line:InvoiceLine;serviceLabel?:string;aliases?:LearnedServiceAlias[];invoice:Invoice}){
  const label=normalizeLabel(line.description)
  const learned=aliases?.find(alias=>alias.normalizedLabel===label&&!alias.invoiceId)
  const invoiceOnly=aliases?.find(alias=>alias.normalizedLabel===label&&alias.invoiceId===invoice.id&&!alias.revokedAt)
  if(!serviceLabel&&!learned)return null
  return <p className="flex flex-wrap items-center gap-x-2 gap-y-1 px-4 pb-1 text-xs text-[var(--text-secondary)] sm:pl-[52px]"><Link2 aria-hidden="true" className="size-3.5"/>{serviceLabel&&<span>Serviciu contractual: „{serviceLabel}”</span>}{invoiceOnly&&!learned&&<span>· asociere numai pentru această factură</span>}{learned&&!learned.revokedAt&&<><span>· formulare învățată pentru acest contract</span><RevokeAliasButton clientId={invoice.clientId} alias={learned} appearance="link"/></>}{learned?.revokedAt&&(invoiceOnly?<span>· formularea învățată a fost revocată; asocierea rămâne pentru această factură</span>:<span>· formulare revocată; următoarea validare va cere din nou asocierea</span>)}</p>
}

function CheckRow({check,invoice,run,reviewable,documentId,onVerify,onMap}:{check:CommercialCheck;invoice:Invoice;run:CommercialValidationRun;reviewable:boolean;documentId?:string;onVerify:(check:CommercialCheck)=>void;onMap:(lineId?:string)=>void}){
  const finding=check.findings[0]
  const overridden=check.findings.every(item=>item.override)
  const outcome=overridden?'CONFORM':check.outcome
  const icon=outcome==='CONFORM'?<CircleCheck aria-hidden="true" className="size-5 text-[var(--success)]"/>:outcome==='NECONFORM'?<CircleX aria-hidden="true" className="size-5 text-[var(--danger)]"/>:<TriangleAlert aria-hidden="true" className="size-5 text-[var(--warning)]"/>
  const merged=check.id.startsWith('vat:')
  const values=check.kind==='comparison'||check.invoiceValue||check.contractValue
  return <div className="border-t border-[var(--border)] first:border-t-0">
    <div className={rowGrid}>
      <span className="hidden sm:flex">{icon}</span>
      <div className="min-w-0"><p className="flex items-center gap-2 text-sm font-semibold"><span className="sm:hidden">{icon}</span><span className="sr-only">{outcomeLabel[outcome]}: </span>{check.title}{merged&&<span className="font-normal text-[var(--text-muted)]">· {check.scope}</span>}</p>{check.explanation&&(check.kind!=='comparison'||outcome!=='CONFORM')&&<p className={`mt-0.5 text-xs ${check.kind==='mapping'&&check.explanation.startsWith('Sugestie')?'text-[var(--text)]':'text-[var(--text-secondary)]'}`}>{check.explanation}</p>}{finding.missingInputs?.length&&check.kind==='input'?<p className="mt-0.5 text-xs text-[var(--warning)]">Date lipsă: {finding.missingInputs.map(missingInputLabel).join(', ')}</p>:null}</div>
      {values?<><Value label="Pe factură" value={check.invoiceValue} detail={check.invoiceDetail}/><Value label="În contract" value={check.contractValue} detail={check.contractDetail}/></>:<span className="hidden sm:col-span-2 sm:block"/>}
      <div className="sm:justify-self-end">
        {check.kind==='comparison'&&<Button type="button" variant="secondary" size="sm" className="text-[var(--accent)]" aria-label={`Verifică ${check.title}${check.scope==='Factură'?'':` · ${check.scope}`}`} onClick={()=>onVerify(check)}>Verifică<ArrowRight aria-hidden="true" className="size-4"/></Button>}
        {check.kind==='mapping'&&reviewable&&!!finding.serviceCandidates?.length&&<Button type="button" variant="secondary" size="sm" className="text-[var(--accent)]" onClick={()=>onMap(finding.lineId)}>Asociază<ArrowRight aria-hidden="true" className="size-4"/></Button>}
        {check.kind==='notice'&&documentId&&check.code==='CONTRACT_COVERAGE_INCOMPLETE'&&<Link className="text-xs font-semibold text-[var(--accent)] underline" to={`/contracts/documents/${encodeURIComponent(invoice.clientId)}/${encodeURIComponent(documentId)}?element=group:clauses`}>Deschide contractul la clauzele de rezolvat</Link>}
      </div>
    </div>
    {finding.override&&check.kind!=='comparison'&&<p className="px-4 pb-2 text-xs text-[var(--success)] sm:pl-[52px]">Excepție aprobată: {finding.override.reason}</p>}
    {reviewable&&run.dossierId&&['RULE_INPUT_MISSING','RULE_INPUT_OUTSIDE_VALIDITY','UNIT_QUANTITY_SOURCE_MISSING'].includes(check.code)&&<div className="px-4 pb-3 sm:pl-[52px]">{finding.missingInputs?.map(name=><MissingVariableForm key={name} invoice={invoice} dossierId={run.dossierId!} name={name} initialValue={finding.actual} initialSource={finding.actualSource}/>)}</div>}
    {reviewable&&check.code==='RULE_DATE_BASIS_MISSING'&&<div className="px-4 pb-3 sm:pl-[52px]">{finding.missingInputs?.map(name=><DateFactForm key={name} invoice={invoice} name={name}/>)}</div>}
    {reviewable&&check.kind!=='comparison'&&check.outcome==='NECONFORM'&&!finding.override&&<ExceptionForm invoice={invoice} run={run} findingId={finding.id} code={check.code}/>}
  </div>
}

function Value({label,value,detail}:{label:string;value?:string;detail?:string}){return <div className="min-w-0 text-sm"><span className="text-xs text-[var(--text-muted)] sm:hidden">{label}: </span><span className="font-medium tabular-nums">{value??'—'}</span>{detail&&<span className="block truncate text-xs text-[var(--text-muted)]" title={detail}>{detail}</span>}</div>}

function ExceptionForm({invoice,run,findingId,code}:{invoice:Invoice;run:CommercialValidationRun;findingId:string;code:string}){
  const resolve=useResolveCommercialValidation(invoice)
  const [reason,setReason]=useState('')
  const act=(action:'ACCEPT_EXCEPTION'|'WAIT_FOR_CORRECTION')=>resolve.mutate({runId:run.id,findingId,expectedInvoiceRevision:invoice.revision??run.invoiceRevision+1,action,reason:action==='ACCEPT_EXCEPTION'?reason.trim():undefined})
  return <div className="px-4 pb-3 sm:pl-[52px]"><textarea aria-label={`Motiv excepție ${code}`} value={reason} onChange={event=>setReason(event.target.value)} placeholder="Motiv obligatoriu pentru aprobarea excepției" className="min-h-16 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] p-2 text-sm"/><div className="mt-2 flex flex-wrap gap-2"><Button type="button" disabled={!reason.trim()||resolve.isPending} onClick={()=>act('ACCEPT_EXCEPTION')}>Acceptă excepția</Button><Button type="button" variant="secondary" disabled={resolve.isPending} onClick={()=>act('WAIT_FOR_CORRECTION')}>Așteaptă corecția</Button></div></div>
}

function DateFactForm({invoice,name}:{invoice:Invoice;name:string}){
  const save=usePutCommercialDateFact(invoice)
  const [date,setDate]=useState('')
  const [sourceReference,setSourceReference]=useState('')
  const kinds={remittance_date:{kind:'REMITTANCE',label:'Data remiterii facturii'},receipt_date:{kind:'RECEIPT',label:'Data primirii facturii'},acceptance_date:{kind:'ACCEPTANCE',label:'Data acceptării serviciului'}} as const
  const item=kinds[name as keyof typeof kinds]
  if(!item)return null
  return <form className="mt-3 rounded-lg border border-[var(--warning)] p-3" onSubmit={event=>{event.preventDefault();if(date&&sourceReference.trim())save.mutate({kind:item.kind,date,sourceReference:sourceReference.trim()})}}><p className="text-xs font-semibold">Completează {item.label.toLocaleLowerCase('ro-RO')} pentru această factură</p><div className="mt-2 grid gap-2 sm:grid-cols-2"><label className="text-xs">{item.label}<input type="date" value={date} onChange={event=>setDate(event.target.value)} className="mt-1 w-full rounded border px-2 py-1"/></label><label className="text-xs">Dovada datei<input value={sourceReference} onChange={event=>setSourceReference(event.target.value)} placeholder="Ex.: confirmare SPV sau mesaj e-mail" className="mt-1 w-full rounded border px-2 py-1"/></label></div><p className="mt-2 text-xs">Introdu numai data dovedită pentru acest eveniment; nu folosi automat data importului sau a emiterii.</p><Button type="submit" variant="secondary" disabled={!date||!sourceReference.trim()||save.isPending||save.isSuccess}>Salvează data cu dovada</Button>{save.isSuccess&&<p className="mt-2 text-xs text-[var(--success)]">Data a fost salvată. Rulează din nou validarea.</p>}{save.isError&&<p role="alert" className="mt-2 text-xs text-[var(--danger)]">Data nu a putut fi salvată.</p>}</form>
}

function OutcomeBadge({outcome}:{outcome:CommercialOutcome}){return <Badge tone={outcome==='CONFORM'?'success':outcome==='NECONFORM'?'danger':'warning'}>{outcomeLabel[outcome]}</Badge>}

function MissingVariableForm({invoice,dossierId,name,initialValue='',initialSource=''}:{invoice:Invoice;dossierId:string;name:string;initialValue?:string;initialSource?:string}){
  const save=usePutCommercialVariable(invoice,dossierId)
  const invoiceDay=invoice.issueDate.slice(0,10)
  const [value,setValue]=useState(initialValue)
  const [sourceReference,setSourceReference]=useState(initialSource)
  const [periodStart,setPeriodStart]=useState('')
  const [periodEnd,setPeriodEnd]=useState('')
  const isQuantity=name.startsWith('unit_quantity_')
  const isVAT=name==='applicable_vat_rate'
  const periodValid=(!periodStart||!periodEnd||periodStart<=periodEnd)&&(!periodStart||periodStart<=invoiceDay)&&(!periodEnd||periodEnd>=invoiceDay)
  const valid=/^-?\d+(?:\.\d+)?$/.test(value)&&sourceReference.trim().length>0&&periodValid&&(!isVAT||!!periodStart)
  return <form className="mt-3 rounded-lg border border-[var(--border)] p-3" onSubmit={event=>{event.preventDefault();if(valid)save.mutate({name,value,source:'MANUAL',sourceReference:sourceReference.trim(),periodStart:periodStart||undefined,periodEnd:periodEnd||undefined})}}>
    <p className="text-xs font-semibold">{isQuantity?'Completează cantitatea serviciului dintr-o sursă verificabilă':isVAT?'Completează cota TVA aplicabilă și sursa fiscală':`Completează valoarea necesară: ${name}`}</p>
    <div className="mt-2 grid gap-2 sm:grid-cols-2"><label className="text-xs">{isVAT?'Cotă TVA (%)':isQuantity?'Cantitate':'Valoare numerică'}<input aria-label={`Valoare ${name}`} value={value} onChange={event=>setValue(event.target.value)} className="mt-1 w-full rounded border px-2 py-1"/></label><label className="text-xs">{isVAT?'Sursă fiscală verificabilă':'Sursă / referință verificabilă'}<input aria-label={`Sursă ${name}`} value={sourceReference} onChange={event=>setSourceReference(event.target.value)} placeholder={isVAT?'Ex.: act normativ și articol':undefined} className="mt-1 w-full rounded border px-2 py-1"/></label><label className="text-xs">Valabilă de la<input type="date" value={periodStart} onChange={event=>setPeriodStart(event.target.value)} className="mt-1 w-full rounded border px-2 py-1"/></label><label className="text-xs">Valabilă până la<input type="date" value={periodEnd} onChange={event=>setPeriodEnd(event.target.value)} className="mt-1 w-full rounded border px-2 py-1"/></label></div>
    {isVAT&&!periodStart&&<p className="mt-2 text-xs text-[var(--warning)]">Precizează data reală de la care această cotă TVA este valabilă.</p>}
    {!periodValid&&<p role="alert" className="mt-2 text-xs text-[var(--danger)]">Perioada trebuie să includă data facturii: {invoiceDay.split('-').reverse().join('.')}.</p>}
    <div className="mt-2 flex items-center gap-2"><Button type="submit" variant="secondary" disabled={!valid||save.isPending}>Salvează cu proveniență</Button>{save.isSuccess&&<span className="text-xs text-[var(--success)]">Salvată. Rulează din nou validarea.</span>}{save.isError&&<span role="alert" className="text-xs text-[var(--danger)]">Valoarea nu a putut fi salvată.</span>}</div>
  </form>
}
