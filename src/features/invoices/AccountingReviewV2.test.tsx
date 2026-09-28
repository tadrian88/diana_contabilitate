import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { RepositoryProvider } from '../../app/repository-context'
import type { ApprovedKnowledge, ClassificationReviewItem, Invoice, PromotionPreview } from '../../domain/invoice'
import { mockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import type { InvoiceRepository } from '../../repositories/invoiceRepository'
import { AccountingReviewV2 } from './AccountingReviewV2'

const dimensions:ClassificationReviewItem['dimension'][]=['ACCOUNT','VAT_TREATMENT','VAT_DEDUCTIBILITY','EXPENSE_TAX_TREATMENT']

function fixture():Invoice {
  const items=dimensions.map((dimension,index):ClassificationReviewItem=>({id:`decision-${index}`,lineId:'line-1',lineLabel:'Abonament Smart 15',dimension,proposedValue:dimension==='ACCOUNT'?'626':'Propunere',proposedTypedValue:dimension==='ACCOUNT'?{kind:'ACCOUNT',account:'626'}:dimension==='VAT_TREATMENT'?{kind:'ORDINARY',timing:'IMMEDIATE',sourceCategory:'S',sourceRate:'21'}:dimension==='VAT_DEDUCTIBILITY'?{kind:'FULL'}:{kind:'FULLY_DEDUCTIBLE'},confidence:'HIGH',explanation:'Decizie propusă pe baza contextului facturii.',legalBasis:'Cod fiscal',legalCitations:[{fragmentId:`f-${index}`,versionId:'2026',citationKey:`Cod fiscal art. ${index+1}`,contentHash:'hash',verified:true}],validationResults:[],status:'PENDING',revision:1,source:'AI_PROPOSAL'}))
  return {id:'invoice-d',scenario:'PROCESSING',primaryDemo:false,clientId:'client-1',supplierName:'Orange România',documentNumber:'OR-1',issueDate:'2026-09-25',total:{amount:97.2,currency:'RON'},spvReference:'SPV',modelVersion:'ACCOUNTING_DOMAIN_V2',pipelineStatus:'AWAITING_REVIEW',pipelinePath:['DOWNLOADED','LINES_READ','CLASSIFIED','AWAITING_REVIEW','READY_FOR_SAGA'],sagaStatus:'NOT_READY',autoRun:false,revision:2,accountingWorkflowStatus:'REVIEW_REQUIRED',activity:[],lines:[{id:'line-1',position:1,description:'Abonament Smart 15',unit:'buc',quantity:1,unitPrice:{amount:80.33,currency:'RON'},netValue:{amount:80.33,currency:'RON'},vatLabel:'21%',vatValue:{amount:16.87,currency:'RON'},grossValue:{amount:97.2,currency:'RON'},classifications:[]}],task:{id:'task-1',type:'CLASSIFICATION',status:'OPEN',createdAt:'2026-09-25T10:00:00Z',title:'Review contabil',reason:'Verifică propunerile contabile.',revision:1,classificationItems:items}}
}

function renderReview(invoice=fixture(), repository:InvoiceRepository=mockInvoiceRepository){const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});return render(<RepositoryProvider repository={repository}><QueryClientProvider client={client}><MemoryRouter><AccountingReviewV2 invoice={invoice}/></MemoryRouter></QueryClientProvider></RepositoryProvider>)}

describe('review contabil V2 unificat',()=>{
  it('afișează CTA-ul de configurare pentru profil lipsă și profil invalid acționabil',()=>{
    const missing=fixture();missing.readinessReason='MISSING_FISCAL_PROFILE';const {unmount}=renderReview(missing)
    expect(screen.getByText(/Profilul fiscal al clientului trebuie configurat/)).toBeInTheDocument()
    expect(screen.getByRole('link',{name:'Vezi profilul fiscal'})).toHaveAttribute('href','/clients/client-1#accounting-profile')
    expect(screen.getByRole('button',{name:'Reanalizează factura'})).toBeInTheDocument()
    unmount();const invalid=fixture();invalid.readinessReason='INVALID_ACCOUNTING_PROFILE';renderReview(invalid)
    expect(screen.getByText(/Configurația contabilă a clientului trebuie corectată/)).toBeInTheDocument()
    expect(screen.getByRole('link',{name:'Remediază profilul'})).toBeInTheDocument()
  })

  it('traduce stările analyzing și completed și nu oferă aprobare la final',()=>{
    const running=fixture();running.accountingWorkflowStatus='AI_ANALYSIS_RUNNING';const {unmount}=renderReview(running)
    expect(screen.getByText('Analiza contabilă este în curs')).toBeInTheDocument();expect(screen.queryByRole('button',{name:/Aprobă toate/})).not.toBeInTheDocument()
    unmount();const completed=fixture();completed.accountingWorkflowStatus='COMPLETED';completed.task=undefined;completed.lines[0].classifications=dimensions.map((dimension,index)=>({id:`f-${index}`,dimension,value:'Final',typedValue:dimension==='ACCOUNT'?{kind:'ACCOUNT',account:'626'}:{kind:'FULL'},explanation:'Final',legalBasis:'Bază',status:'ACCEPTED',humanReviewed:true,effectiveSource:'MANUAL'}));renderReview(completed)
    expect(screen.getByText('Clasificare contabilă finalizată')).toBeInTheDocument();expect(screen.queryByRole('button',{name:/Aprobă/})).not.toBeInTheDocument()
  })

  it('prezintă cele patru dimensiuni, sumarul, sursa și citarea lângă decizie',async()=>{
    const user=userEvent.setup();renderReview()
    for(const label of ['Cont contabil','Tratament TVA','Drept de deducere TVA','Tratament fiscal al cheltuielii — impozit pe profit'])expect(screen.getByLabelText(label)).toBeInTheDocument()
    expect(screen.getByText('4 de verificat')).toBeInTheDocument();expect(screen.getAllByText('Sursă: Propunere automată')).toHaveLength(4)
    await user.click(screen.getAllByText('Vezi baza legală')[0]);expect(screen.getByText('Cod fiscal art. 1')).toBeInTheDocument()
    expect(screen.queryByLabelText(/JSON/)).not.toBeInTheDocument();expect(screen.queryByText('Analiză contabilă asistată')).not.toBeInTheDocument()
  })

  it.each([
    ['ACCOUNT_NOT_FOUND','Contul propus nu există în planul de conturi.'],
    ['ACCOUNT_INACTIVE','Contul există, dar este inactiv.'],
    ['ACCOUNT_NOT_ALLOWED_BY_PROFILE','Contul nu este disponibil în configurația contabilă a acestui client.'],
  ])('traduce eroarea %s', (code,message)=>{const invoice=fixture();invoice.task!.classificationItems![0].validationResults=[{code,message:'technical'}];renderReview(invoice);expect(screen.getByText(message)).toBeInTheDocument();expect(screen.queryByText(code)).not.toBeInTheDocument()})

  it('explică 628, păstrează TVA valid și oferă analiticul selectabil',async()=>{
    const invoice=fixture();invoice.task!.classificationItems![0].proposedTypedValue={kind:'ACCOUNT',account:'628'};invoice.task!.classificationItems![0].validationResults=[{code:'ACCOUNT_NOT_POSTABLE',message:'invalid',suggestedAccounts:['6281']}];renderReview(invoice)
    expect(screen.getByText(/Contul este sintetic și nu poate fi utilizat direct/)).toBeInTheDocument();expect(await screen.findByRole('button',{name:/Selectează contul 6281/})).toBeInTheDocument()
    expect(screen.getByLabelText('Tratament TVA')).toHaveTextContent('TVA obișnuită');expect(screen.getByText('3 de verificat')).toBeInTheDocument();expect(screen.getByText('1 de rezolvat')).toBeInTheDocument()
  })

  it('marchează explicit citarea neverificată',async()=>{const user=userEvent.setup();const invoice=fixture();invoice.task!.classificationItems![1].legalCitations![0].verified=false;renderReview(invoice);await user.click(screen.getAllByText('Vezi baza legală')[1]);expect(screen.getByText('Referința legală nu a putut fi verificată.')).toBeInTheDocument()})

  it('aprobă individual prin contractul cu revizii și păstrează respingerea ca acțiune secundară',async()=>{
    const user=userEvent.setup();const invoice=fixture();const repository=Object.create(mockInvoiceRepository) as InvoiceRepository;const review=vi.fn(async()=>invoice);repository.reviewClassification=review;renderReview(invoice,repository)
    await user.click(screen.getAllByRole('button',{name:'Aprobă'})[0]);await waitFor(()=>expect(review).toHaveBeenCalled());expect((review.mock.calls as unknown[][])[0][7]).toBe('APPROVE')
    await user.click(screen.getAllByRole('button',{name:'Respinge'})[0]);await user.type(screen.getByLabelText('Motivul respingerii'),'Nu corespunde documentelor');await user.click(screen.getByRole('button',{name:'Respinge propunerea'}));await waitFor(()=>expect(review).toHaveBeenLastCalledWith(expect.anything(),expect.anything(),undefined,undefined,'Nu corespunde documentelor',undefined,undefined,'REJECT'))
  })

  it('aprobă în grup numai propunerile valide și explică un conflict CAS',async()=>{
    const user=userEvent.setup();const invoice=fixture();invoice.task!.classificationItems![0].validationResults=[{code:'ACCOUNT_NOT_POSTABLE',message:'invalid'}]
    const repository=Object.create(mockInvoiceRepository) as InvoiceRepository;const approve=vi.fn(async()=>invoice);repository.approveAllClassifications=approve;const {unmount}=renderReview(invoice,repository)
    await user.click(screen.getByRole('button',{name:'Aprobă toate propunerile valide'}));await waitFor(()=>expect(approve).toHaveBeenCalledWith('invoice-d'));expect(screen.getByText('3 de verificat')).toBeInTheDocument();expect(screen.getByText('1 de rezolvat')).toBeInTheDocument()
    unmount();const conflictRepository=Object.create(mockInvoiceRepository) as InvoiceRepository;conflictRepository.approveAllClassifications=vi.fn(async()=>Promise.reject({response:{status:409}}));renderReview(fixture(),conflictRepository)
    await user.click(screen.getByRole('button',{name:'Aprobă toate propunerile valide'}));expect(await screen.findByText(/Factura a fost modificată între timp/)).toBeInTheDocument();expect(screen.getByRole('button',{name:'Reîncarcă'})).toBeInTheDocument()
  })

  it('arată fallback-ul manual după failure și permite modificarea structurată',async()=>{const user=userEvent.setup();const invoice=fixture();invoice.task!.reason='Analiza asistată nu este disponibilă; completați manual clasificările.';delete invoice.task!.classificationItems![2].proposedTypedValue;renderReview(invoice);expect(screen.getByText('Analiza automată nu a putut fi finalizată.')).toBeInTheDocument();await user.click(screen.getByLabelText('Drept de deducere TVA').querySelector('button')!);expect(screen.getByRole('dialog')).toBeInTheDocument()})

  it('blochează review-ul stale și oferă reanalizarea ca acțiune principală',()=>{const invoice=fixture();invoice.classificationContext={runId:'old',profileVersion:1,contextStale:true,staleReasons:['FISCAL_PROFILE_CHANGED'],createdAt:'2026-09-24'};renderReview(invoice);expect(screen.getByText(/Configurația clientului s-a modificat/)).toBeInTheDocument();expect(screen.getByRole('button',{name:'Reanalizează factura'})).toBeInTheDocument();expect(screen.queryByRole('button',{name:'Aprobă'})).not.toBeInTheDocument()})

  it('oferă reuse numai după decizia finală și cere confirmarea scope-ului exact',async()=>{const user=userEvent.setup();const invoice=fixture();invoice.task=undefined;invoice.accountingWorkflowStatus='COMPLETED';invoice.currentClassificationRunId='run-current';invoice.classificationContext={runId:'run-current',contextStale:false,staleReasons:[],createdAt:'2026-09-25'};invoice.lines[0].classifications=dimensions.map((dimension,index)=>({id:`final-${index}`,dimension,value:dimension==='ACCOUNT'?'626':'FULL',typedValue:dimension==='ACCOUNT'?{kind:'ACCOUNT',account:'626'}:{kind:'FULL'},explanation:'Final',legalBasis:'Bază',status:'ACCEPTED',humanReviewed:true,effectiveSource:'MANUAL',revision:2}));const repository=Object.create(mockInvoiceRepository) as InvoiceRepository;repository.previewApprovedKnowledge=vi.fn(async():Promise<PromotionPreview>=>({classificationId:'final-0',classificationRevision:2,classificationRunId:'run-current',dimension:'ACCOUNT',value:{kind:'ACCOUNT',account:'626'},scope:{clientId:'client-1',clientDisplay:'AUDIPREST',supplierDisplay:'Orange România',normalizedSupplierId:'orange',serviceIdentityKind:'NORMALIZED_DESCRIPTION',serviceIdentityValue:'abonament smart 15',normalizerVersion:'NORMALIZED_DESCRIPTION_V1',currency:'RON',documentType:'INVOICE',vatRate:'21',profileId:'profile',profileVersion:1}}));repository.promoteApprovedKnowledge=vi.fn(async(_client:string,_invoice:string,preview:PromotionPreview):Promise<ApprovedKnowledge>=>({id:'knowledge-1',version:1,dimension:preview.dimension,value:preview.value,scope:preview.scope,status:'ACTIVE',sourceInvoiceId:'invoice-d',sourceInvoiceLineId:'line-1',sourceClassificationId:'final-0',sourceClassificationRunId:'run-current',originalSource:'MANUAL',promotedBy:'Contabil',promotedAt:'2026-09-25T10:00:00Z',revision:1}));renderReview(invoice,repository);expect(screen.getAllByRole('button',{name:'Folosește pentru situații similare'})).toHaveLength(4);await user.click(screen.getAllByRole('button',{name:'Folosește pentru situații similare'})[0]);expect(await screen.findByRole('dialog',{name:'Confirmă reutilizarea deciziei'})).toHaveTextContent('Descriere normalizată = abonament smart 15');await user.click(screen.getByRole('button',{name:'Confirmă reutilizarea'}));await waitFor(()=>expect(repository.promoteApprovedKnowledge).toHaveBeenCalled());expect(screen.getByText(/acum reutilizabilă/)).toBeInTheDocument()})
})
