import { useCallback, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { ContractDocument, ContractExtractionSafeErrorCategory } from '../../repositories/invoiceRepository'
import { ContractPDFPreview } from './ContractPDFPreview'
import { useContractDocument, useDiscardContractDocument, useRetryContractExtraction } from './contract-ingestion-hooks'
import { ConfirmedContractWorkspace } from './ConfirmedContractWorkspace'
import { ExtractionHistory } from './contract-review-parts'
import { ProposalReviewWorkspace } from './ProposalReviewWorkspace'

const transientFailures = new Set<ContractExtractionSafeErrorCategory>(['PROVIDER_RATE_LIMIT','PROVIDER_UNAVAILABLE','PROVIDER_NETWORK','TIMEOUT','INTERRUPTED','LEASE_EXPIRED','PROVIDER_TRANSIENT'])
const invalidOutputFailures = new Set<ContractExtractionSafeErrorCategory>(['INVALID_PROVIDER_ENVELOPE','INVALID_STRUCTURED_OUTPUT','PROPOSAL_VALIDATION_FAILED','INVALID_OUTPUT'])
function extractionFailureMessage(category?:ContractExtractionSafeErrorCategory){
  if(category&&transientFailures.has(category))return 'Serviciul de extragere nu a răspuns. Reîncearcă.'
  if(category&&invalidOutputFailures.has(category))return 'Răspunsul automat nu a putut fi validat. Reîncearcă extragerea.'
  if(category==='EXTRACTOR_CONFIGURATION'||category==='PROVIDER_AUTHENTICATION'||category==='PROVIDER_REJECTED'||category==='SOURCE_INTEGRITY')return 'Serviciul de extragere necesită intervenția administratorului.'
  if(category==='NO_CONTRACT_DATA')return 'Documentul nu conține suficiente date contractuale pentru extragere.'
  return 'Nu am putut extrage automat datele contractului.'
}

const inProgress=(document:ContractDocument)=>document.status==='UPLOADED'||document.status==='EXTRACTING'||document.status==='EXTRACTION_FAILED'

export function ContractDocumentReviewPage(){
  const {clientId='',documentId=''}=useParams();const {data:document,isLoading,isError,refetch}=useContractDocument(clientId,documentId);const [params]=useSearchParams();const [page,setPage]=useState(1);const refreshProposal=useCallback(()=>{void refetch()},[refetch])
  return <div className="space-y-5"><Link to="/contracts" className="text-sm font-semibold text-[var(--accent)]">Înapoi la Contracte</Link>{params.get('duplicate')==='true'&&<p role="status">Acest PDF există deja pentru client. Nu a fost creat un duplicat și nu s-a repetat extragerea.</p>}{isLoading?<p role="status">Se încarcă documentul…</p>:isError?<p role="alert">Documentul nu a putut fi încărcat.</p>:!document?<p role="alert">Documentul nu a fost găsit.</p>:document.status==='CONFIRMED'?<ConfirmedContractWorkspace document={document}/>:inProgress(document)?<><header><h2 className="text-xl font-bold">Verifică datele extrase</h2><p className="mt-1 text-sm">{document.originalFilename} · Client: {document.clientName??document.clientId}</p></header><div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]"><ContractPDFPreview clientId={clientId} documentId={documentId} page={page} onPage={setPage}/><ContractProposalReview document={document} onStale={refreshProposal}/></div></>:<ContractProposalReview key={`${document.id}:${document.extraction?.id}:${document.revision}`} document={document} onStale={refreshProposal}/>}</div>
}

// A document in review: its extraction status while Diana reads it, then the
// side-by-side review of what was read; a confirmed document shows what it
// enforces now.
export function ContractProposalReview({document,onStale}:{document:ContractDocument;onStale:()=>void}){
  const navigate=useNavigate();const retry=useRetryContractExtraction(document.clientId,document.id);const discard=useDiscardContractDocument(document.clientId,document.id)
  const onDiscarded=()=>navigate(`/contracts/upload?clientId=${document.clientId}`)
  if(document.status==='UPLOADED'||document.status==='EXTRACTING')return <section className="card p-6"><Badge tone="info">În procesare</Badge><p role="status" className="mt-4">Diana extrage automat datele contractului. Poți reveni mai târziu; starea este păstrată.</p><DiscardButton document={document} discard={discard} onDone={onDiscarded}/></section>
  if(document.status==='EXTRACTION_FAILED')return <section className="card p-6"><Badge tone="warning">Extragere eșuată</Badge><p role="alert" className="mt-4">{extractionFailureMessage(document.extraction?.safeErrorCategory)}</p><div className="mt-4 flex gap-2"><Button disabled={retry.isPending} onClick={()=>retry.mutate(document.revision)}>Reîncearcă extragerea</Button><DiscardButton document={document} discard={discard} onDone={onDiscarded}/></div><ExtractionHistory document={document}/></section>
  if(document.status==='CONFIRMED')return <ConfirmedContractWorkspace document={document}/>
  return <ProposalReviewWorkspace document={document} onStale={onStale} footer={<footer className="card flex flex-wrap items-start justify-between gap-4 px-5 py-4"><div className="min-w-0 flex-1"><p className="text-sm text-[var(--text-secondary)]">Verifică propunerea în raport cu originalul. Valorile editate sunt candidatele autoritative; propunerea AI rămâne în istoric. Nimic nu devine contract până nu confirmi.</p><ExtractionHistory document={document}/></div><DiscardButton document={document} discard={discard} onDone={onDiscarded}/></footer>}/>
}

function DiscardButton({document,discard,onDone}:{document:ContractDocument;discard:ReturnType<typeof useDiscardContractDocument>;onDone:()=>void}){return <Button type="button" variant="secondary" disabled={discard.isPending} onClick={()=>{if(window.confirm('Renunți la acest document neconfirmat? Facturile vor rămâne în așteptarea contractului.'))discard.mutate(document.revision,{onSuccess:onDone})}}>{discard.isPending?'Se elimină…':'Renunță / Șterge documentul'}</Button>}
