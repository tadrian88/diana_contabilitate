import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { vi } from 'vitest'
import type { ContractDocument } from '../../repositories/invoiceRepository'
import { ContractProposalReview } from './ContractDocumentReviewPage'

const handlers = vi.hoisted(() => ({ confirm: vi.fn(() => Promise.resolve({ contractId: 'contract-1', changed: true })), confirmRule: vi.fn(), revise: vi.fn(), activatePrices: vi.fn(), retry: vi.fn(), discard: vi.fn(), dismiss: vi.fn() }))
// The PDF text index: every snippet found exactly, unless a test scans the PDF.
const pdfText = vi.hoisted(() => ({ scanned: false, location: 'EXACT' as 'EXACT' | 'PARTIAL' | 'NONE' }))
vi.mock('./ContractPDFPreview', () => ({ ContractPDFPreview: ({page,highlights}:{page:number;highlights?:string[]}) => <div data-testid="contract-pdf">Private PDF preview · pagina {page} · {(highlights??[]).join(' | ')}</div> }))
vi.mock('./useContractPdf', () => ({ useContractPdf: () => ({ pdf: null, error: '', retry: vi.fn(), ready: true, scanned: pdfText.scanned, locate: () => ({ location: pdfText.location, page: 1 }) }) }))
vi.mock('./contract-ingestion-hooks', () => ({
  useConfirmContractDocument: () => ({ mutateAsync: handlers.confirm, isPending: false, isError: false }),
  useRetryContractExtraction: () => ({ mutate: handlers.retry, isPending: false, isError: false }),
  useDiscardContractDocument: () => ({ mutate: handlers.discard, isPending: false, isError: false }),
  useContractDocument: vi.fn(),
}))
vi.mock('./contract-hooks', () => ({useConfirmProposedCommercialRule:()=>({mutate:handlers.confirmRule,isPending:false,isError:false}),useReviseConfirmedCommercialRule:()=>({mutate:handlers.revise,isPending:false,isError:false}),useActivateReviewedServicePrices:()=>({mutate:handlers.activatePrices,isPending:false,isError:false,isSuccess:false}),useCommercialServiceAliases:()=>({data:[]}),useRevokeCommercialServiceAlias:()=>({mutate:vi.fn(),isPending:false,isError:false}),useDismissProposedCommercialClause:()=>({mutate:handlers.dismiss,isPending:false,isError:false})}))

function fixture(): ContractDocument {
  const field = (value: string) => ({ value, status: 'PRESENT' as const, confidence: 'HIGH' as const, evidence: { page: 1, snippet: value }, alternatives: [] })
  return {
    id: 'document-1', clientId: 'client-1', originalFilename: 'contract.pdf', mimeType: 'application/pdf', sizeBytes: 500,
    sha256: 'synthetic-hash', status: 'READY_FOR_REVIEW', lifecycleState: 'ACTIVE', revision: 3,
    uploadedAt: '2026-09-15T12:00:00Z', uploadedBy: 'Uploader', buyerMismatch: false,
    extraction: { id: 'attempt-1', provider: 'DETERMINISTIC_TEST', model: 'fixture-v1', schemaVersion: 'CONTRACT_EXTRACTION_V1',
      promptVersion: 'CONTRACT_EXTRACTION_PROMPT_V1', status: 'SUCCEEDED', startedAt: '2026-09-15T12:00:01Z',
      proposal: { supplierName: field('Supplier SRL'), supplierCui: field('RO12345678'), reference: field('AI-REFERENCE'),
        effectiveFrom: field('2026-01-01'), effectiveTo: field('2027-12-31'), totalValue: field('125000.00'), currency: field('RON'),
        unitType: field('servicii'), paymentTerms: field('30 zile'), buyerCui: field('RO10000000'), periodType:field('FIXED_TERM'), documentRole:field('BASE_CONTRACT'),serviceTerms:[] } },
  }
}
function review(document = fixture(), path = '/') {
  render(<MemoryRouter initialEntries={[path]}><ContractProposalReview document={document} onStale={vi.fn()} /></MemoryRouter>)
}
const row = (name: RegExp) => screen.getByRole('button', { name })
const active = (name: RegExp) => expect(row(name)).toHaveAttribute('aria-current', 'true')
const confirmedValues=(values:Partial<NonNullable<ContractDocument['confirmedValues']>>={}):NonNullable<ContractDocument['confirmedValues']>=>({supplierName:'Supplier SRL',supplierCui:'RO12345678',buyerCui:'RO10000000',reference:'AI-REFERENCE',effectiveFrom:'2026-01-01',effectiveTo:'2027-12-31',totalValue:'125000',currency:'RON',unitType:'servicii',paymentTerms:'30 zile',periodType:'FIXED_TERM',documentRole:'BASE_CONTRACT',relatedReference:'',coverage:'PARTIAL',serviceTerms:[],commercialRules:[],...values})
function confirmedFixture(values:Partial<NonNullable<ContractDocument['confirmedValues']>>={}){const document=fixture();document.status='CONFIRMED';document.confirmedContractId='contract-1';document.confirmedAt='2026-09-16T10:00:00Z';document.confirmedValues=confirmedValues(values);return document}
const clause=(id:string,kind:string,text:string,expression:NonNullable<ContractDocument['confirmedValues']>['commercialRules'][number]['expression']=null)=>({kind:{value:kind,status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:1,snippet:text},alternatives:[]},narrative:{value:text,status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:1,snippet:text},alternatives:[]},evidence:{page:1,snippet:text},confidence:'HIGH' as const,rule:{id,kind,narrative:text,applicability:{},dateBasis:'',expression,evidence:[{documentId:'',page:1,snippet:text}],blocking:false}})
beforeEach(() => { vi.clearAllMocks(); pdfText.scanned = false; pdfText.location = 'EXACT' })

describe('Contract ingestion human review boundary', () => {
  it('shows the proposal without creating a contract automatically', () => {
    review()
    expect(row(/Referință contract.*AI-REFERENCE/)).toBeInTheDocument()
    expect(screen.getByText('Valorile editate sunt candidatele autoritative', { exact: false })).toBeInTheDocument()
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('submits edited values and the exact extraction revision only on explicit confirmation', async () => {
    const user = userEvent.setup(); const document = fixture(); review(document)
    await user.click(row(/Referință contract/))
    await user.click(screen.getByRole('button', { name: 'Corectează' }))
    await user.clear(screen.getByLabelText('Referință contract'))
    await user.type(screen.getByLabelText('Referință contract'), 'USER-CORRECTION')
    expect(handlers.confirm).not.toHaveBeenCalled()
    expect(row(/Referință contract/)).toHaveTextContent('Corectat de tine')
    await user.click(screen.getByRole('button', { name: 'Confirmă contractul' }))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({ document, key: expect.any(String), contract: expect.objectContaining({ reference: 'USER-CORRECTION' }) }))
    expect(document.extraction?.proposal?.reference.value).toBe('AI-REFERENCE')
  })
  it('opens the PDF at the words of the selected item independently of confirmation', async () => {
    const document = fixture(); document.extraction!.proposal!.effectiveFrom = { ...document.extraction!.proposal!.effectiveFrom, evidence: { page: 2, snippet: 'începând cu 01.01.2026' } }
    review(document)
    await userEvent.setup().click(row(/Data de început/))
    expect(await screen.findByTestId('contract-pdf')).toHaveTextContent('pagina 2 · începând cu 01.01.2026')
    expect(row(/Data de început/)).toHaveTextContent('Verificat automat')
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('does not invent a missing currency and blocks confirmation on its row', async () => {
    const document = fixture(); document.extraction!.proposal!.currency = { value: null, status: 'MISSING', confidence: 'UNKNOWN', evidence: { page: null, snippet: '' }, alternatives: [] }
    review(document)
    active(/Monedă/)
    expect(screen.getByLabelText('Monedă ISO')).toHaveValue('')
    expect(within(screen.getByRole('alert')).getByText('Moneda ISO este obligatorie.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Confirmă contractul' })).not.toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Mai ai 1 de verificat' }))
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('blocks confirmation when authoritative unit type or payment terms are missing', () => {
    const document=fixture();document.extraction!.proposal!.unitType={...document.extraction!.proposal!.unitType,value:null,status:'MISSING'};document.extraction!.proposal!.paymentTerms={...document.extraction!.proposal!.paymentTerms,value:null,status:'MISSING'}
    review(document)
    expect(screen.getByRole('alert')).toHaveTextContent('Tipul unității / baza comercială este obligatoriu.')
    expect(screen.getByRole('alert')).toHaveTextContent('Termenii de plată sunt obligatorii.')
    expect(screen.getByRole('button',{name:'Mai ai 2 de verificat'})).toBeInTheDocument()
    expect(screen.queryByRole('button',{name:'Confirmă contractul'})).not.toBeInTheDocument()
  })
  it('keeps incomplete service pricing empty for human review',()=>{
    const document=fixture();const missing={value:null,status:'MISSING' as const,confidence:'UNKNOWN' as const,evidence:{page:null,snippet:''},alternatives:[]}
    document.extraction!.proposal!.serviceTerms=[{serviceDescription:{...missing,value:'Contabilitate',status:'PRESENT',confidence:'HIGH',evidence:{page:2,snippet:'Contabilitate'}},pricingModel:missing,unitPrice:missing,currency:missing,unit:missing,quantitySource:missing,quantityValue:missing,quantityDriver:missing,billingFrequency:missing}]
    review(document)
    active(/Contabilitate/)
    expect(screen.getByLabelText('Model tarifare 1')).toHaveValue('')
    expect(screen.getByLabelText('Preț 1')).toHaveValue('')
    expect(within(screen.getByRole('alert')).getByText('Serviciul 1: modelul de tarifare este obligatoriu.')).toBeInTheDocument()
  })
  it('retains a clause without an expression as partial review, not an executable price rule',async()=>{
    const document=fixture();const extracted={value:'Tarif după grila de documente',status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:'Tarif după grila de documente'},alternatives:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'TIERED_PRICE'},narrative:extracted,evidence:{page:2,snippet:'Tarif după grila de documente'},confidence:'HIGH',rule:{id:'pending-grid',kind:'TIERED_PRICE',narrative:'Tarif după grila de documente',applicability:{},dateBasis:'INVOICE_ISSUE_DATE',expression:null,evidence:[{documentId:'document-1',page:2,snippet:'Tarif după grila de documente'}],blocking:true}}]
    const user=userEvent.setup();review(document)
    expect(row(/Tarif după grila de documente/)).toHaveTextContent('Se rezolvă după confirmare')
    await user.click(row(/Acoperire comercială/))
    expect(screen.getByLabelText('Acoperire comercială')).toBeDisabled()
    await user.click(screen.getByRole('button',{name:'Confirmă contractul'}))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({contract:expect.objectContaining({coverage:'PARTIAL',commercialRules:[]})}))
  })
  it('requires explicit human confirmation for a normalized AI rule',async()=>{
    const document=fixture();const extracted={value:'Tarif fix de 500 RON',status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:'Tarif fix de 500 RON'},alternatives:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'FIXED_PRICE'},narrative:extracted,evidence:{page:2,snippet:'Tarif fix de 500 RON'},confidence:'HIGH',rule:{id:'fixed-500',kind:'FIXED_PRICE',narrative:'Tarif fix de 500 RON',applicability:{},dateBasis:'INVOICE_ISSUE_DATE',expression:{op:'literal',value:'500'},evidence:[{documentId:'document-1',page:2,snippet:'Tarif fix de 500 RON'}],blocking:true}}]
    const user=userEvent.setup();review(document)
    active(/Tarif fix de 500 RON/)
    expect(row(/Tarif fix de 500 RON/)).toHaveTextContent('De verificat: regulă propusă de AI')
    expect(screen.getByText('Prețul unitar de pe factură trebuie să fie 500.')).toBeInTheDocument()
    expect(screen.getByText(/"op"/)).not.toBeVisible()
    expect(screen.queryByRole('button',{name:'Confirmă contractul'})).not.toBeInTheDocument()
    // Leaving it for after confirmation keeps today's behaviour: the contract is confirmed without it.
    await user.click(screen.getByRole('button',{name:'Lasă pentru după confirmare'}))
    await user.click(screen.getByRole('button',{name:'Confirmă contractul'}))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({contract:expect.objectContaining({coverage:'PARTIAL',commercialRules:[]})}))
    handlers.confirm.mockClear()
    await user.click(screen.getByRole('button',{name:'Confirmă regula'}))
    expect(row(/Tarif fix de 500 RON/)).toHaveTextContent('Inclusă în confirmare')
    await user.click(row(/Acoperire comercială/))
    expect(screen.getByLabelText('Acoperire comercială')).toBeEnabled()
    await user.selectOptions(screen.getByLabelText('Acoperire comercială'),'COMPLETE')
    await user.click(screen.getByRole('button',{name:'Confirmă contractul'}))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({contract:expect.objectContaining({coverage:'COMPLETE',commercialRules:[expect.objectContaining({id:'fixed-500',expression:{op:'literal',value:'500'}})]})}))
  })
  it('allows an executable proposal to be confirmed after the contract was already confirmed',async()=>{
    const document=fixture();const extracted={value:'Tarif fix de 500 RON',status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:'Tarif fix de 500 RON'},alternatives:[]}
    document.status='CONFIRMED';document.confirmedContractId='contract-1';document.confirmedAt='2026-09-16T10:00:00Z';document.confirmedValues={supplierName:'Supplier SRL',supplierCui:'RO12345678',buyerCui:'RO10000000',reference:'AI-REFERENCE',effectiveFrom:'2026-01-01',effectiveTo:'2027-12-31',totalValue:'125000',currency:'RON',unitType:'servicii',paymentTerms:'30 zile',periodType:'FIXED_TERM',documentRole:'BASE_CONTRACT',relatedReference:'',coverage:'PARTIAL',serviceTerms:[],commercialRules:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'FIXED_PRICE'},narrative:extracted,evidence:{page:2,snippet:'Tarif fix de 500 RON'},confidence:'HIGH',rule:{id:'fixed-500',kind:'FIXED_PRICE',narrative:'Tarif fix de 500 RON',applicability:{},dateBasis:'INVOICE_ISSUE_DATE',expression:{op:'literal',value:'500'},evidence:[{documentId:'document-1',page:2,snippet:'Tarif fix de 500 RON'}],blocking:true}}]
    review(document)
    await userEvent.setup().click(screen.getByRole('button',{name:'Confirmă regula'}))
    expect(handlers.confirmRule).toHaveBeenCalledWith({ruleId:'fixed-500'})
  })
  it('shows which confirmed tariffs invoices are checked against and activates the unused one',async()=>{
    const term=(serviceDescription:string,unitPrice:string,unit:string)=>({serviceDescription,pricingModel:'FIXED_FEE' as const,unitPrice,currency:'RON',unit,quantitySource:'UNKNOWN' as const,quantityValue:'',quantityDriver:'',billingFrequency:'MONTHLY' as const,evidence:{page:1}})
    const document=confirmedFixture({reference:'BG-2025-117',periodType:'INDEFINITE_TERM',effectiveTo:'',serviceTerms:[term('Mentenanță IT — abonament lunar','1800.00','lună'),term('Hosting cloud — pachet Business 2 VM','650.00','lună'),term('Intervenții suplimentare','175.00','oră')]})
    document.commercialState={dossierId:'dossier-1',snapshotVersion:3,coverage:'PARTIAL',activeRuleIds:['service-hosting'],services:[{position:1,active:false},{position:2,active:true,ruleId:'service-hosting'},{position:3,active:false,skipReason:'PRICE_CHANGED_FROM_SOURCE'}],clauses:[]}
    review(document)
    expect(screen.getByText('Folosit la verificarea facturilor')).toBeInTheDocument()
    expect(screen.getByText('Nefolosit: prețul confirmat diferă de cel citat din PDF; corectează-l sau reextrage')).toBeInTheDocument()
    expect(screen.getByRole('button',{name:/Mentenanță IT — abonament lunar/})).toHaveAttribute('aria-current','true')
    expect(screen.getByText(/Un tarif confirmat nu este încă folosit la verificarea facturilor/)).toBeInTheDocument()
    expect(await screen.findByTestId('contract-pdf')).toHaveTextContent('pagina 1')
    await userEvent.setup().click(screen.getByRole('button',{name:'Activează tarifele confirmate'}))
    expect(handlers.activatePrices).toHaveBeenCalled()
  })
  it('says the tariff state is unknown instead of guessing when it could not be read',()=>{
    const document=confirmedFixture({serviceTerms:[{serviceDescription:'Contabilitate',pricingModel:'FIXED_FEE',unitPrice:'500',currency:'RON',unit:'lună',quantitySource:'UNKNOWN',quantityValue:'',quantityDriver:'',billingFrequency:'MONTHLY',evidence:{page:1}}]})
    review(document)
    expect(screen.getByText('Stare necunoscută')).toBeInTheDocument()
    expect(screen.getByText('Acoperire necunoscută')).toBeInTheDocument()
    expect(screen.queryByRole('button',{name:'Activează tarifele confirmate'})).not.toBeInTheDocument()
  })
  it('explains confirmed commercial rules in plain language and keeps the technical formula collapsed',async()=>{
    const base={applicability:{},evidence:[{documentId:'document-1',page:2,snippet:'sursă'}],blocking:true}
    const document=confirmedFixture({reference:'102/25.06.2025',paymentTerms:'5 zile',totalValue:'',commercialRules:[
      {...base,id:'fixed',kind:'FIXED_PRICE',narrative:'servicii de contabilitate · 500 RON',dateBasis:'INVOICE_ISSUE_DATE',currency:'RON',expression:{op:'literal',value:'500',scale:4}},
      {...base,id:'reference',kind:'CONTRACT_REFERENCE',narrative:'Referința contractului confirmat este 102/25.06.2025',dateBasis:'INVOICE_ISSUE_DATE',expression:{op:'literal',value:'102/25.06.2025',scale:4}},
      {...base,id:'due',kind:'PAYMENT_DUE',narrative:'Plata se va efectua în termen de 5 zile de la data remiterii facturii.',dateBasis:'RECEIPT_DATE',expression:{op:'literal',value:'5',scale:2}},
      {...base,id:'vat',kind:'VAT',narrative:'Prețurile sunt fără TVA',dateBasis:'INVOICE_ISSUE_DATE',expression:{op:'variable',variable:'applicable_vat_rate'},requiredVariables:['applicable_vat_rate']},
    ]})
    review(document)
    expect(screen.getByText('Prețul unitar de pe factură trebuie să fie 500 RON.')).toBeInTheDocument()
    expect(screen.getByText('Factura trebuie să menționeze contractul „102/25.06.2025”.')).toBeInTheDocument()
    expect(screen.getByText('Scadența facturii trebuie să fie la 5 zile de la primirea / remiterea facturii.')).toBeInTheDocument()
    expect(screen.getByText('Termen de plată')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button',{name:/Se aplică cota TVA legală valabilă la data facturii/}))
    expect(screen.getByText(/applicable_vat_rate/)).not.toBeVisible()
    expect(await screen.findByTestId('contract-pdf')).toHaveTextContent('pagina 2 · sursă')
  })
  it('allows a reviewer to complete a payment term without changing its source evidence',async()=>{
    const document=fixture();const extracted={value:'Plata în 5 zile de la remiterea facturii',status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:'Plata în 5 zile de la remiterea facturii'},alternatives:[]}
    document.status='CONFIRMED';document.confirmedContractId='contract-1';document.confirmedAt='2026-09-16T10:00:00Z';document.confirmedValues={supplierName:'Supplier SRL',supplierCui:'RO12345678',buyerCui:'RO10000000',reference:'AI-REFERENCE',effectiveFrom:'2026-01-01',effectiveTo:'2027-12-31',totalValue:'125000',currency:'RON',unitType:'servicii',paymentTerms:'5 zile',periodType:'FIXED_TERM',documentRole:'BASE_CONTRACT',relatedReference:'',coverage:'PARTIAL',serviceTerms:[],commercialRules:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'PAYMENT_DUE'},narrative:extracted,evidence:{page:2,snippet:extracted.value},confidence:'HIGH',rule:{id:'payment-5',kind:'PAYMENT_DUE',narrative:extracted.value!,applicability:{},dateBasis:'',expression:null,evidence:[{documentId:'',page:2,snippet:extracted.value!}],blocking:true}}]
    review(document);const user=userEvent.setup();expect(screen.getByLabelText('Număr de zile payment-5')).toHaveValue('5');expect(screen.getByLabelText('Bază termen payment-5')).toHaveValue('RECEIPT_DATE');await user.click(screen.getByRole('button',{name:'Confirmă regula completată'}))
    expect(handlers.confirmRule).toHaveBeenCalledWith({ruleId:'payment-5',rule:expect.objectContaining({id:'payment-5',kind:'PAYMENT_DUE',dateBasis:'RECEIPT_DATE',expression:{op:'literal',value:'5',scale:2}})})
  })
  it('treats a legal-rate VAT clause as applicable VAT even when it cites the rate at signing',()=>{
    const document=fixture();const text='Prețurile nu includ TVA. TVA se aplică în cota legală în vigoare (21% la data semnării).';const extracted={value:text,status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:1,snippet:text},alternatives:[]}
    document.status='CONFIRMED';document.confirmedContractId='contract-1';document.confirmedAt='2026-09-16T10:00:00Z';document.confirmedValues={supplierName:'Supplier SRL',supplierCui:'RO12345678',buyerCui:'RO10000000',reference:'AI-REFERENCE',effectiveFrom:'2026-01-01',effectiveTo:'2027-12-31',totalValue:'',currency:'RON',unitType:'servicii',paymentTerms:'',periodType:'FIXED_TERM',documentRole:'BASE_CONTRACT',relatedReference:'',coverage:'PARTIAL',serviceTerms:[],commercialRules:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'VAT'},narrative:extracted,evidence:{page:1,snippet:text},confidence:'HIGH',rule:{id:'vat-legal',kind:'VAT',narrative:text,applicability:{},dateBasis:'',expression:null,evidence:[{documentId:'',page:1,snippet:text}],blocking:false}}]
    review(document)
    expect(screen.getByRole('button',{name:'Confirmă „TVA aplicabilă”'})).toBeInTheDocument()
    expect(screen.queryByLabelText('Cotă TVA (%) vat-legal')).not.toBeInTheDocument()
  })
  it('announces clauses recognized from source text before contract confirmation',()=>{
    const document=fixture();const text='în termen de 10 zile calendaristice de la data emiterii facturii';const extracted={value:text,status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:text},alternatives:[]}
    const rule={id:'payment-10',kind:'PAYMENT_DUE',narrative:text,applicability:{},dateBasis:'',expression:null,evidence:[{documentId:'',page:2,snippet:text}],blocking:false}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'PAYMENT_DUE'},narrative:extracted,evidence:{page:2,snippet:text},confidence:'HIGH',rule,recognizedRule:{...rule,dateBasis:'INVOICE_ISSUE_DATE',expression:{op:'literal',value:'10',scale:2},blocking:true,origin:'SOURCE_TEXT'}}]
    review(document)
    expect(screen.getByText(/Recunoscută automat din text/)).toBeInTheDocument()
    expect(screen.getByText('Scadența facturii trebuie să fie la 10 zile de la data emiterii facturii.')).toBeInTheDocument()
    expect(screen.queryByText('Se rezolvă după confirmare')).not.toBeInTheDocument()
    expect(screen.getByRole('button',{name:'Confirmă contractul'})).toBeEnabled()
  })
  it('lets the reviewer edit a rule confirmed automatically from source text',async()=>{
    const document=fixture();const text='în termen de 10 zile calendaristice de la data emiterii facturii';const extracted={value:text,status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:text},alternatives:[]}
    const proposed={id:'payment-10',kind:'PAYMENT_DUE',narrative:text,applicability:{},dateBasis:'',expression:null,evidence:[{documentId:'',page:2,snippet:text}],blocking:false}
    const confirmedRule={...proposed,dateBasis:'INVOICE_ISSUE_DATE',expression:{op:'literal' as const,value:'10',scale:2},evidence:[{documentId:'document-1',page:2,snippet:text}],blocking:true,origin:'SOURCE_TEXT' as const}
    document.status='CONFIRMED';document.confirmedContractId='contract-1';document.confirmedAt='2026-09-16T10:00:00Z';document.confirmedValues={supplierName:'Supplier SRL',supplierCui:'RO12345678',buyerCui:'RO10000000',reference:'AI-REFERENCE',effectiveFrom:'2026-01-01',effectiveTo:'2027-12-31',totalValue:'',currency:'RON',unitType:'servicii',paymentTerms:'',periodType:'FIXED_TERM',documentRole:'BASE_CONTRACT',relatedReference:'',coverage:'PARTIAL',serviceTerms:[],commercialRules:[confirmedRule]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'PAYMENT_DUE'},narrative:extracted,evidence:{page:2,snippet:text},confidence:'HIGH',rule:proposed}]
    review(document);const user=userEvent.setup()
    await user.click(screen.getByRole('button',{name:/Termen de plată/}))
    expect(screen.getByText('Recunoscută automat din textul clauzei')).toBeInTheDocument()
    expect(screen.queryByText('Clauze comerciale neconfirmate')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button',{name:'Modifică valorile'}))
    const days=screen.getByLabelText('Valoare modificată payment-10');expect(days).toHaveValue('10');await user.clear(days);await user.type(days,'15')
    await user.selectOptions(screen.getByLabelText('Bază termen modificată payment-10'),'RECEIPT_DATE')
    await user.click(screen.getByRole('button',{name:'Salvează modificarea'}))
    expect(handlers.revise).toHaveBeenCalledWith({ruleId:'payment-10',rule:expect.objectContaining({id:'payment-10',dateBasis:'RECEIPT_DATE',expression:{op:'literal',value:'15',scale:2}})},expect.anything())
  })
  it('confirms applicable VAT treatment without inventing a percentage absent from the contract',async()=>{
    const document=fixture()
    const extracted={value:'Prețurile sunt fără TVA, la ele se adaugă TVA aferent.',status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:'Prețurile sunt fără TVA, la ele se adaugă TVA aferent.'},alternatives:[]}
    document.status='CONFIRMED';document.confirmedContractId='contract-1';document.confirmedAt='2026-09-16T10:00:00Z';document.confirmedValues={supplierName:'Supplier SRL',supplierCui:'RO12345678',buyerCui:'RO10000000',reference:'AI-REFERENCE',effectiveFrom:'2026-01-01',effectiveTo:'2027-12-31',totalValue:'125000',currency:'RON',unitType:'servicii',paymentTerms:'5 zile',periodType:'FIXED_TERM',documentRole:'BASE_CONTRACT',relatedReference:'',coverage:'PARTIAL',serviceTerms:[],commercialRules:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'VAT'},narrative:extracted,evidence:{page:2,snippet:extracted.value},confidence:'HIGH',rule:{id:'vat-unknown',kind:'VAT',narrative:extracted.value!,applicability:{},dateBasis:'',expression:null,evidence:[{documentId:'document-1',page:2,snippet:extracted.value!}],blocking:true}}]
    review(document)
    expect(screen.getByText(/nu fixează un procent/i)).toBeInTheDocument()
    expect(screen.queryByLabelText('Cotă TVA (%) vat-unknown')).not.toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button',{name:'Confirmă „TVA aplicabilă”'}))
    expect(handlers.confirmRule).toHaveBeenCalledWith({ruleId:'vat-unknown',rule:expect.objectContaining({kind:'VAT',expression:{op:'variable',variable:'applicable_vat_rate'},requiredVariables:['applicable_vat_rate']})})
  })
  it('closes an identity clause that only names the supplier CUI in one click',async()=>{
    const document=confirmedFixture()
    document.extraction!.proposal!.commercialClauses=[clause('identity-1','IDENTITY','Supplier SRL, J40/123/2020, cod de înregistrare fiscală RO12345678')]
    review(document)
    expect(screen.getByText('De rezolvat')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button',{name:'Închide: acoperită de CUI-ul RO12345678'}))
    expect(handlers.dismiss).toHaveBeenCalledWith({ruleId:'identity-1',reasonCode:'COVERED_BY_SUPPLIER_IDENTITY'})
  })
  it('closes the client\'s own identity clause in one click, labelled as the client',async()=>{
    const document=confirmedFixture()
    document.extraction!.proposal!.commercialClauses=[clause('identity-3','IDENTITY','SOFTCO2 S.R.L., J2024004283403, cod de înregistrare fiscală RO10000000, denumită BENEFICIAR')]
    review(document)
    expect(screen.getByRole('button',{name:/Identitate client/})).toBeInTheDocument()
    expect(screen.queryByRole('button',{name:/Identitate furnizor/})).not.toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button',{name:'Închide: acoperită de CUI-ul clientului RO10000000'}))
    expect(handlers.dismiss).toHaveBeenCalledWith({ruleId:'identity-3',reasonCode:'COVERED_BY_PARTY_IDENTITY'})
  })
  it('closes a clause naming both parties as covered by their CUIs',async()=>{
    const document=confirmedFixture()
    document.extraction!.proposal!.commercialClauses=[clause('identity-2','IDENTITY','Prestator RO12345678, Beneficiar RO10000000')]
    review(document)
    await userEvent.setup().click(screen.getByRole('button',{name:'Închide: acoperită de CUI-urile părților (RO12345678, RO10000000)'}))
    expect(handlers.dismiss).toHaveBeenCalledWith({ruleId:'identity-2',reasonCode:'COVERED_BY_PARTY_IDENTITY'})
  })
  it('does not offer the identity shortcut when the clause names a company that is not a party',()=>{
    const document=confirmedFixture()
    document.extraction!.proposal!.commercialClauses=[clause('identity-4','IDENTITY','Prestator RO12345678, semnat pentru RO55555555')]
    review(document)
    expect(screen.queryByRole('button',{name:/acoperită de CUI/})).not.toBeInTheDocument()
    expect(screen.getByRole('button',{name:/Identitate parte/})).toBeInTheDocument()
    expect(screen.getByRole('button',{name:'Închide clauza: nu influențează facturile…'})).toBeInTheDocument()
  })
  it('closes a clause that is not checked on invoices only with a reason, warning for VAT',async()=>{
    const document=confirmedFixture();const user=userEvent.setup()
    document.extraction!.proposal!.commercialClauses=[clause('vat-fixed','VAT','TVA 19% pentru serviciile prestate în 2024.')]
    review(document)
    await user.click(screen.getByRole('button',{name:'Închide clauza: nu influențează facturile…'}))
    expect(screen.getByText(/toate facturile contractului rămân în verificare comercială/)).toBeInTheDocument()
    expect(screen.getByText('Atenție: termenii acestei clauze nu vor mai fi verificați pe facturi.')).toBeInTheDocument()
    expect(screen.getByRole('button',{name:'Închide clauza'})).toBeDisabled()
    await user.type(screen.getByLabelText('De ce nu influențează facturile?'),'Clauză istorică, înlocuită de cota legală')
    await user.click(screen.getByRole('button',{name:'Închide clauza'}))
    expect(handlers.dismiss).toHaveBeenCalledWith({ruleId:'vat-fixed',reasonCode:'NOT_INVOICE_VERIFIABLE',reason:'Clauză istorică, înlocuită de cota legală'})
  })
  it('shows a closed clause with why and by whom, and the coverage from the active version',()=>{
    const document=confirmedFixture()
    document.extraction!.proposal!.commercialClauses=[clause('identity-1','IDENTITY','cod de înregistrare fiscală RO12345678')]
    document.commercialState={dossierId:'dossier-1',snapshotVersion:4,coverage:'COMPLETE',activeRuleIds:[],services:[],clauses:[{ruleId:'identity-1',kind:'IDENTITY',status:'REJECTED',reasonCode:'COVERED_BY_SUPPLIER_IDENTITY',reason:'Furnizorul este identificat prin CUI-ul confirmat.',reviewedBy:'ana@firma.ro',reviewedAt:'2026-09-29T10:00:00Z'}]}
    review(document)
    expect(screen.getByText('Acoperire completă')).toBeInTheDocument()
    expect(screen.getByText('Versiunea 4')).toBeInTheDocument()
    expect(screen.getByText('Acoperită de CUI-ul furnizorului')).toBeInTheDocument()
    expect(screen.queryByText('De rezolvat')).not.toBeInTheDocument()
  })
  it('opens the element an invoice check links to and moves with the arrow keys',async()=>{
    const base={applicability:{},evidence:[{documentId:'document-1',page:2,snippet:'Plata în 15 zile'}],blocking:true}
    const document=confirmedFixture({commercialRules:[{...base,id:'due',kind:'PAYMENT_DUE',narrative:'Plata în 15 zile',dateBasis:'INVOICE_ISSUE_DATE',expression:{op:'literal',value:'15',scale:2}}]})
    review(document,'/?element=rule:due')
    expect(screen.getByRole('button',{name:/Termen de plată/})).toHaveAttribute('aria-current','true')
    expect(await screen.findByTestId('contract-pdf')).toHaveTextContent('pagina 2 · Plata în 15 zile')
    await userEvent.setup().keyboard('{ArrowUp}')
    expect(screen.getByRole('button',{name:/Termeni de plată/})).toHaveAttribute('aria-current','true')
  })
  it('blocks buyer mismatch rather than silently moving the document to another client', () => {
    const document=fixture();document.clientCui='RO10000000';document.extraction!.proposal!.buyerCui= {...document.extraction!.proposal!.buyerCui,value:'RO99999999'};review(document)
    expect(screen.queryByRole('button', { name: 'Confirmă contractul' })).not.toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('CUI-ul cumpărătorului')
    active(/CUI cumpărător/)
  })
  it('recomputes buyer blocker from the edited reviewed value', async()=>{
    const document=fixture();document.clientCui='RO21592770';document.extraction!.proposal!.buyerCui={...document.extraction!.proposal!.buyerCui,value:'RO99999999'};review(document)
    const input=screen.getByLabelText('CUI cumpărător');await userEvent.setup().clear(input);await userEvent.setup().type(input,'ro 21592770')
    expect(screen.getByRole('button',{name:'Confirmă contractul'})).toBeEnabled()
  })
  it('models indefinite term without an end date',async()=>{
    const user=userEvent.setup();review()
    await user.click(row(/Durata/));await user.click(screen.getByRole('button',{name:'Corectează'}));await user.click(screen.getByLabelText('Nedeterminată'))
    expect(screen.queryByLabelText('Data de sfârșit')).not.toBeInTheDocument();expect(screen.getByRole('button',{name:'Confirmă contractul'})).toBeEnabled()
  })
  it('keeps failure recovery out of the manual-from-zero form', async () => {
    review({ ...fixture(), status: 'EXTRACTION_FAILED' })
    expect(screen.queryByLabelText('Referință contract')).not.toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Reîncearcă extragerea' }))
    expect(handlers.retry).toHaveBeenCalledWith(3)
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it.each([
    ['TIMEOUT','Serviciul de extragere nu a răspuns. Reîncearcă.'],
    ['INVALID_STRUCTURED_OUTPUT','Răspunsul automat nu a putut fi validat. Reîncearcă extragerea.'],
    ['PROVIDER_AUTHENTICATION','Serviciul de extragere necesită intervenția administratorului.'],
    ['NO_CONTRACT_DATA','Documentul nu conține suficiente date contractuale pentru extragere.'],
  ] as const)('shows an actionable safe message for %s',(category,message)=>{
    const document=fixture();document.status='EXTRACTION_FAILED';document.extraction!.status='FAILED';document.extraction!.safeErrorCategory=category;review(document)
    expect(screen.getByRole('alert')).toHaveTextContent(message)
  })
  it('shows the safe category in extraction history without provider diagnostics',async()=>{
    const document=fixture();document.status='EXTRACTION_FAILED';document.extraction!.status='FAILED';document.extraction!.safeErrorCategory='PROVIDER_REJECTED';review(document)
    await userEvent.setup().click(screen.getByText('Istoric și proveniență extragere'))
    expect(screen.getByText(/PROVIDER_REJECTED/)).toBeInTheDocument()
  })
  it('shows processing as persisted state without a blank manual form', () => {
    review({ ...fixture(), status: 'EXTRACTING' })
    expect(screen.getByRole('status')).toHaveTextContent('extrage automat')
    expect(screen.queryByLabelText('Referință contract')).not.toBeInTheDocument()
  })
})

describe('Contract review with pre-verified items', () => {
  const medium = (document: ContractDocument, key: 'unitType' | 'paymentTerms') => { document.extraction!.proposal![key] = { ...document.extraction!.proposal![key], confidence: 'MEDIUM' } }
  const service = (description: string, price: string, snippet: string) => {
    const read = (value: string, evidence: string) => ({ value, status: 'PRESENT' as const, confidence: 'HIGH' as const, evidence: { page: 1, snippet: evidence }, alternatives: [] })
    const missing = { value: null, status: 'MISSING' as const, confidence: 'UNKNOWN' as const, evidence: { page: null }, alternatives: [] }
    return { serviceDescription: read(description, description), pricingModel: read('FIXED_FEE', snippet), unitPrice: read(price, snippet), currency: read('RON', snippet), unit: missing, quantitySource: missing, quantityValue: missing, quantityDriver: missing, billingFrequency: read('MONTHLY', snippet) }
  }

  it('leaves a medium-confidence value to check and confirms only after the reviewer ticks it', async () => {
    const document = fixture(); medium(document, 'unitType'); const user = userEvent.setup(); review(document)
    active(/Tip unitate/)
    expect(row(/Tip unitate/)).toHaveTextContent('De verificat: încredere medie')
    expect(row(/Furnizor/)).toHaveTextContent('Verificat automat: găsit exact în PDF')
    expect(screen.getByText('10 din 11 verificate · 1 de verificat')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Confirmă contractul' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /^Corect\s*↵$/ }))
    expect(row(/Tip unitate/)).toHaveTextContent('Verificat de tine')
    await user.click(screen.getByRole('button', { name: 'Confirmă contractul' }))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({ contract: expect.objectContaining({ unitType: 'servicii' }) }))
  })

  it('ticks the item with Enter and moves to the next one to check', async () => {
    const document = fixture(); medium(document, 'unitType'); medium(document, 'paymentTerms'); review(document)
    active(/Tip unitate/)
    await userEvent.setup().keyboard('{Enter}')
    expect(row(/Tip unitate/)).toHaveTextContent('Verificat de tine')
    active(/Termeni de plată/)
    expect(screen.getByRole('button', { name: 'Mai ai 1 de verificat' })).toBeInTheDocument()
  })

  it('restores a value with Escape while correcting it', async () => {
    const user = userEvent.setup(); review()
    await user.click(row(/Referință contract/))
    await user.keyboard('e')
    const input = screen.getByLabelText('Referință contract')
    await user.clear(input); await user.type(input, 'GREȘIT{Escape}')
    expect(screen.queryByLabelText('Referință contract')).not.toBeInTheDocument()
    expect(row(/Referință contract/)).toHaveTextContent('AI-REFERENCE')
    expect(row(/Referință contract/)).toHaveTextContent('Verificat automat')
  })

  it('shows one banner for a scanned PDF and leaves every item to the reviewer', () => {
    pdfText.scanned = true; pdfText.location = 'NONE'
    review()
    expect(screen.getAllByText(/PDF-ul nu are text/)).toHaveLength(1)
    expect(screen.getByRole('button', { name: 'Mai ai 11 de verificat' })).toBeInTheDocument()
    expect(screen.queryByText('Verificat automat: găsit exact în PDF')).not.toBeInTheDocument()
  })

  it('does not tick a value the cited words do not state', () => {
    const document = fixture(); document.extraction!.proposal!.totalValue = { ...document.extraction!.proposal!.totalValue, evidence: { page: 1, snippet: 'Valoarea totală a contractului este 12.500,00 lei' } }
    review(document)
    expect(row(/Valoare contractuală totală/)).toHaveTextContent('De verificat: valoarea nu apare în fragment')
  })

  it('opens the element a link names', () => {
    const document = fixture(); medium(document, 'unitType')
    review(document, '/?element=field:currency')
    active(/Monedă/)
  })

  it('keeps each service paired with its own proposal row when one is removed', async () => {
    const document = fixture(); document.extraction!.proposal!.serviceTerms = [service('Mentenanță IT', '1800.00', 'Mentenanță IT 1.800,00 lei / lună'), service('Hosting cloud', '650.00', 'Hosting cloud 650,00 lei / lună')]
    const user = userEvent.setup(); review(document)
    expect(row(/Hosting cloud/)).toHaveTextContent('Verificat automat')
    await user.click(row(/Mentenanță IT/))
    await user.click(screen.getByRole('button', { name: 'Elimină' }))
    expect(row(/Hosting cloud/)).toHaveTextContent('Verificat automat')
    await user.click(screen.getByRole('button', { name: 'Confirmă contractul' }))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({ contract: expect.objectContaining({ serviceTerms: [expect.objectContaining({ serviceDescription: 'Hosting cloud', sourceIndex: 1 })] }) }))
  })

  it('adds a service the extraction missed, without evidence', async () => {
    const user = userEvent.setup(); review()
    await user.click(screen.getByRole('button', { name: 'Adaugă serviciu' }))
    expect(screen.getByLabelText('Serviciu 1')).toHaveFocus()
    await user.type(screen.getByLabelText('Serviciu 1'), 'Backup')
    await user.selectOptions(screen.getByLabelText('Model tarifare 1'), 'FIXED_FEE')
    await user.type(screen.getByLabelText('Preț 1'), '100')
    await user.click(screen.getByRole('button', { name: 'Confirmă contractul' }))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({ contract: expect.objectContaining({ serviceTerms: [expect.objectContaining({ serviceDescription: 'Backup', unitPrice: '100', currency: 'RON', sourceIndex: -1 })] }) }))
  })
})

describe('lease where the client is the Locator (D-126)', () => {
  function leaseFixture(withTenantName = true) {
    const document = fixture()
    const proposal = document.extraction!.proposal!
    document.clientCui = 'RO10000000'
    proposal.supplierCui = { ...proposal.supplierCui, value: 'RO10000000', evidence: { page: 1, snippet: 'RO10000000' } }
    proposal.buyerCui = { ...proposal.buyerCui, value: 'RO40138380', evidence: { page: 1, snippet: 'RO40138380' } }
    if (withTenantName) proposal.buyerName = { ...proposal.supplierName, value: 'Chiriaș Test SRL', evidence: { page: 1, snippet: 'Chiriaș Test SRL' } }
    return document
  }
  it('confirms with the client as supplier and the tenant as buyer', async () => {
    const document = leaseFixture(); review(document)
    expect(row(/Cumpărător \(locatar\)/)).toHaveTextContent('Chiriaș Test SRL')
    expect(screen.queryByText(/nu corespunde clientului selectat/)).not.toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Confirmă contractul' }))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({ contract: expect.objectContaining({ supplierCui: 'RO10000000', buyerCui: 'RO40138380', buyerName: 'Chiriaș Test SRL' }) }))
  })
  it('requires the tenant name when the client is the supplier', () => {
    review(leaseFixture(false))
    expect(row(/Cumpărător \(locatar\)/)).toHaveTextContent('Denumirea cumpărătorului (locatarului) este obligatorie')
    expect(screen.queryByRole('button', { name: 'Confirmă contractul' })).not.toBeInTheDocument()
  })
  it('does not ask for a buyer name on a purchase contract', () => {
    const document = fixture(); document.clientCui = 'RO10000000'; review(document)
    expect(screen.queryByRole('button', { name: /Cumpărător \(locatar\)/ })).not.toBeInTheDocument()
  })
})
