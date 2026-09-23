import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { Invoice } from '../../domain/invoice'
import type { CommercialFinding, CommercialOutcome } from '../../repositories/invoiceRepository'
import { useCommercialValidation, useConfirmCommercialServiceAlias, usePutCommercialDateFact, usePutCommercialVariable, useResolveCommercialValidation } from './invoice-hooks'

const outcomeLabel:Record<CommercialOutcome,string>={CONFORM:'Conform',NECONFORM:'Neconform',NEVERIFICABIL:'Nu poate fi verificat'}
const findingLabel:Record<string,string>={
  CONTRACT_REFERENCE_MISSING:'Referința contractului nu a fost găsită',CONTRACT_REFERENCE_MISMATCH:'Factura indică alt contract',CONTRACT_REFERENCE_MATCH:'Referința contractului este corectă',
  CONTRACT_COVERAGE_INCOMPLETE:'Unele condiții contractuale nu pot fi încă verificate',SERVICE_LINE_UNCOVERED:'Asociază linia facturii cu serviciul contractual',SERVICE_LINE_UNMATCHED:'Confirmă serviciul corespunzător acestei formulări',SERVICE_LINE_AMBIGUOUS:'Linia se potrivește cu mai multe servicii',
  PRICE_MATCH:'Prețul este corect',PRICE_MISMATCH:'Prețul diferă de contract',PAYMENT_DUE_MATCH:'Scadența este corectă',PAYMENT_DUE_MISMATCH:'Scadența diferă de contract',
  ORIGINAL_INVOICE_UNAVAILABLE:'Factura inițială pentru storno nu este disponibilă',RULE_INPUT_MISSING:'Lipsesc date necesare calculului',RULE_INPUT_OUTSIDE_VALIDITY:'Informația este salvată, dar nu este valabilă la data facturii',CURRENCY_MISMATCH:'Moneda diferă de contract',LINE_PRICE_CURRENCY_MISSING:'Moneda prețului nu poate fi verificată',
  CONTRACT_NOT_EFFECTIVE:'Contractul nu este valabil la data facturii',
  RULE_DATE_BASIS_MISSING:'Lipsește data de la care începe termenul de plată',
  UNIT_QUANTITY_SOURCE_MISSING:'Lipsește dovada cantității facturate',UNIT_QUANTITY_MATCH:'Cantitatea facturată este corectă',UNIT_QUANTITY_MISMATCH:'Cantitatea facturată diferă de dovadă',
}
const actionExplanation:Record<string,string>={
  CONTRACT_COVERAGE_INCOMPLETE:'Unele clauze nu au o valoare verificabilă din contract. Tarifele confirmate pot fi totuși comparate separat.',
  SERVICE_LINE_UNCOVERED:'Dacă tarifele sunt active, alege serviciul din contract la care se referă această linie. Nu folosi suma facturii pentru alegere.',
  RULE_DATE_BASIS_MISSING:'Contractul stabilește termenul de plată de la transmiterea, primirea sau acceptarea facturii. Completează data evenimentului cerut și dovada ei; data importului nu o înlocuiește.',
  RULE_INPUT_OUTSIDE_VALIDITY:'Valoarea completată anterior a fost salvată, însă intervalul ales nu include data acestei facturi. Corectează perioada de valabilitate, fără să schimbi valoarea sau sursa dacă acestea sunt corecte.',
}
const missingInputLabel=(name:string)=>({remittance_date:'data remiterii facturii',receipt_date:'data primirii facturii',acceptance_date:'data acceptării serviciului',invoice_due_date:'scadența facturii'}[name]??(name.startsWith('unit_quantity_')?'cantitatea serviciului':name))

export function CommercialValidationCard({invoice}:{invoice:Invoice}){
  const {data:run,isLoading}=useCommercialValidation(invoice)
  const resolve=useResolveCommercialValidation(invoice)
  const [reasons,setReasons]=useState<Record<string,string>>({})
  if(['DOWNLOADED','ARCHIVED','MATCHING','AWAITING_CONTRACT','AWAITING_MATCH_CONFIRM','DEDUPE_CHECKED','HEADER_READ','LINES_READ','DUPLICATE'].includes(invoice.pipelineStatus))return null
  if(isLoading)return <section className="card p-5" role="status">Se încarcă validarea comercială…</section>
  if(!run)return <section className="card p-5"><h3 className="font-bold">Validare comercială</h3><p className="mt-2 text-sm text-[var(--text-secondary)]">Rezultatul nu este disponibil încă.</p></section>
  const act=(finding:CommercialFinding|undefined,action:'ACCEPT_EXCEPTION'|'WAIT_FOR_CORRECTION'|'RERUN')=>resolve.mutate({runId:run.id,findingId:finding?.id,expectedInvoiceRevision:invoice.revision??run.invoiceRevision+1,action,reason:finding?reasons[finding.id]:undefined})
  const needsServiceAlias=run.findings.some(finding=>finding.code==='SERVICE_LINE_UNCOVERED'&&!!finding.serviceCandidates?.length)
  const needsContractReview=!needsServiceAlias&&run.findings.some(finding=>finding.code==='CONTRACT_COVERAGE_INCOMPLETE'||finding.code==='SERVICE_LINE_UNCOVERED')
  const documentId=run.findings.flatMap(finding=>finding.evidence??[]).find(evidence=>evidence.documentId)?.documentId
  return <section className="card p-5" aria-labelledby="commercial-title"><div className="flex items-center justify-between gap-4"><div><p className="eyebrow">Contract Intelligence</p><h3 id="commercial-title" className="mt-1 font-bold">Validare comercială</h3></div><OutcomeBadge outcome={run.outcome}/></div>
    {invoice.pipelineStatus==='AWAITING_COMMERCIAL_REVIEW'&&needsContractReview&&<div className="mt-4 rounded-lg border border-[var(--warning)] bg-[var(--surface)] p-4"><h4 className="text-sm font-bold">Ce ai de făcut acum</h4><ol className="mt-2 list-inside list-decimal space-y-2 text-sm"><li>Deschide contractul. Dacă tarifele apar deja la „Servicii și tarife confirmate”, apasă „Activează tarifele confirmate”. Pentru alte clauze, confirmă numai valori scrise în PDF; nu prelua sume sau cote TVA din factură. {documentId?<Link className="font-semibold text-[var(--accent)] underline" to={`/contracts/documents/${encodeURIComponent(invoice.clientId)}/${encodeURIComponent(documentId)}`}>Deschide contractul pentru confirmare</Link>:<span>Deschide documentul contractual din pagina Contracte.</span>}</li>{needsServiceAlias&&<li>În formularul de mai jos, confirmă serviciul la care se referă „{invoice.lines[0]?.description??'descrierea de pe factură'}”.</li>}<li>Revino la această factură și apasă „Rulează din nou după completări”.</li>{!needsServiceAlias&&<li>Dacă apare apoi solicitarea de asociere a serviciului, confirmă ce înseamnă descrierea de pe factură pentru acest contract.</li>}</ol><p className="mt-2 text-xs text-[var(--text-secondary)]">Nu introduce motiv de excepție pentru informații încă neconfirmate. Referința contractului este deja identificată.</p></div>}
    <details className="mt-2 text-xs text-[var(--text-muted)]"><summary>Detalii tehnice</summary>Motor {run.engineVersion} · snapshot v{run.snapshotVersion || 'legacy parțial'}</details>
    <div className="mt-4 space-y-3">{run.findings.map(finding=><article key={finding.id} className="rounded-lg border border-[var(--border)] p-4"><div className="flex items-start justify-between gap-4"><div><div className="text-sm font-bold">{findingLabel[finding.code]??finding.code}</div><p className="mt-1 text-sm text-[var(--text-secondary)]">{actionExplanation[finding.code]??finding.reason}</p></div><OutcomeBadge outcome={finding.outcome}/></div>
      {(finding.actual||finding.expected)&&<dl className="mt-3 grid gap-2 text-xs sm:grid-cols-2"><div><dt className="text-[var(--text-muted)]">Actual</dt><dd className="font-semibold">{finding.actual||'—'}</dd></div><div><dt className="text-[var(--text-muted)]">Așteptat</dt><dd className="font-semibold">{finding.expected||'—'}</dd></div></dl>}
      {finding.actualSource&&<p className="mt-2 text-xs text-[var(--text-muted)]">{finding.code==='RULE_INPUT_OUTSIDE_VALIDITY'?'Sursa valorii salvate':'Sursă în factură'}: {finding.actualSource}</p>}
      {finding.calculation&&<p className="mt-2 text-xs">{finding.code==='RULE_INPUT_OUTSIDE_VALIDITY'?'Perioada salvată':'Calcul'}: {finding.calculation}</p>}{finding.missingInputs?.length?<p className="mt-2 text-xs text-[var(--warning)]">Date lipsă: {finding.missingInputs.map(missingInputLabel).join(', ')}</p>:null}
      {invoice.pipelineStatus==='AWAITING_COMMERCIAL_REVIEW'&&run.dossierId&&['RULE_INPUT_MISSING','RULE_INPUT_OUTSIDE_VALIDITY','UNIT_QUANTITY_SOURCE_MISSING'].includes(finding.code)&&finding.missingInputs?.map(name=><MissingVariableForm key={name} invoice={invoice} dossierId={run.dossierId!} name={name} initialValue={finding.actual} initialSource={finding.actualSource}/>)}
      {invoice.pipelineStatus==='AWAITING_COMMERCIAL_REVIEW'&&finding.code==='RULE_DATE_BASIS_MISSING'&&finding.missingInputs?.map(name=><DateFactForm key={name} invoice={invoice} name={name}/>)}
      {invoice.pipelineStatus==='AWAITING_COMMERCIAL_REVIEW'&&finding.code==='SERVICE_LINE_UNCOVERED'&&!!finding.serviceCandidates?.length&&<ServiceAliasForm invoice={invoice} finding={finding}/>} 
      {!!finding.evidence?.length&&<details className="mt-2 text-xs text-[var(--text-muted)]"><summary>Vezi fragmentul din contract</summary>{finding.evidence.map((evidence,index)=><p key={`${evidence.documentId}-${index}`} className="mt-1">{evidence.page?`Pagina ${evidence.page} · `:''}{evidence.snippet}</p>)}</details>}
      {finding.override?<p className="mt-3 text-xs text-[var(--success)]">Excepție aprobată: {finding.override.reason}</p>:invoice.pipelineStatus==='AWAITING_COMMERCIAL_REVIEW'&&finding.outcome==='NECONFORM'?<div className="mt-3"><textarea aria-label={`Motiv excepție ${finding.code}`} value={reasons[finding.id]??''} onChange={event=>setReasons(old=>({...old,[finding.id]:event.target.value}))} placeholder="Motiv obligatoriu pentru aprobarea excepției" className="min-h-20 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] p-2 text-sm"/><div className="mt-2 flex flex-wrap gap-2"><Button type="button" disabled={!reasons[finding.id]?.trim()||resolve.isPending} onClick={()=>act(finding,'ACCEPT_EXCEPTION')}>Acceptă excepția</Button><Button type="button" variant="secondary" disabled={resolve.isPending} onClick={()=>act(finding,'WAIT_FOR_CORRECTION')}>Așteaptă corecția</Button></div></div>:null}
    </article>)}</div>
    {invoice.pipelineStatus==='AWAITING_COMMERCIAL_REVIEW'&&<div className="mt-4"><Button type="button" variant="secondary" disabled={resolve.isPending} onClick={()=>act(undefined,'RERUN')}>Rulează din nou după completări</Button></div>}
  </section>
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

function ServiceAliasForm({invoice,finding}:{invoice:Invoice;finding:CommercialFinding}){
  const save=useConfirmCommercialServiceAlias(invoice)
  const [lineId,setLineId]=useState(finding.lineId??invoice.lines[0]?.id??'')
  const [serviceId,setServiceId]=useState('')
  const [reuseForDossier,setReuseForDossier]=useState(false)
  const line=invoice.lines.find(item=>item.id===lineId)
  const label=line?.description.trim()??''
  return <div className="mt-3 rounded-lg border border-[var(--warning)] p-3"><p className="text-xs font-semibold">Ce serviciu din contract reprezintă această linie?</p>{invoice.lines.length>1&&<label className="mt-2 block text-xs">Linia facturii<select aria-label={`Linie pentru ${finding.ruleId}`} value={lineId} onChange={event=>setLineId(event.target.value)} className="mt-1 w-full rounded border px-2 py-1">{invoice.lines.map(item=><option key={item.id} value={item.id}>{item.position}. {item.description}</option>)}</select></label>}<p className="mt-2 text-xs">Formulare de pe factură: <span className="font-semibold">{label||'Indisponibilă'}</span></p><label className="mt-2 block text-xs">Serviciul contractual<select aria-label={`Serviciul contractual pentru linia ${line?.position??1}`} value={serviceId} onChange={event=>setServiceId(event.target.value)} className="mt-1 w-full rounded border px-2 py-1"><option value="">Alege serviciul din contract</option>{finding.serviceCandidates?.map(candidate=><option key={candidate.ruleId} value={candidate.ruleId}>{candidate.label}</option>)}</select></label><p className="mt-2 text-xs">Alege după descrierea serviciului și contract, nu după valoarea facturii.</p><label className="mt-2 flex items-start gap-2 text-xs"><input type="checkbox" checked={reuseForDossier} onChange={event=>setReuseForDossier(event.target.checked)}/>Reutilizează asocierea pe facturile viitoare numai dacă această formulare înseamnă întotdeauna același serviciu pentru acest contract.</label><Button type="button" variant="secondary" disabled={!label||!lineId||!serviceId||save.isPending||save.isSuccess} onClick={()=>save.mutate({serviceId,lineId,reuseForDossier})}>{save.isSuccess?'Asociere salvată':'Confirmă asocierea'}</Button>{save.isSuccess&&<p className="mt-2 text-xs text-[var(--success)]">Asocierea a fost salvată {reuseForDossier?'pentru acest contract':'numai pentru această factură'}. Rulează din nou validarea.</p>}{save.isError&&<p role="alert" className="mt-2 text-xs text-[var(--danger)]">Asocierea nu a putut fi salvată.</p>}</div>
}

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
