import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { vi } from 'vitest'
import type { ContractDocument } from '../../repositories/invoiceRepository'
import { ContractProposalReview } from './ContractDocumentReviewPage'

const handlers = vi.hoisted(() => ({ confirm: vi.fn(), confirmRule: vi.fn(), activatePrices: vi.fn(), retry: vi.fn(), discard: vi.fn() }))
vi.mock('./ContractPDFPreview', () => ({ ContractPDFPreview: () => <div>Private PDF preview</div> }))
vi.mock('./contract-ingestion-hooks', () => ({
  useConfirmContractDocument: () => ({ mutate: handlers.confirm, isPending: false, isError: false }),
  useRetryContractExtraction: () => ({ mutate: handlers.retry, isPending: false, isError: false }),
  useDiscardContractDocument: () => ({ mutate: handlers.discard, isPending: false, isError: false }),
  useContractDocument: vi.fn(),
}))
vi.mock('./contract-hooks', () => ({useConfirmProposedCommercialRule:()=>({mutate:handlers.confirmRule,isPending:false,isError:false}),useActivateReviewedServicePrices:()=>({mutate:handlers.activatePrices,isPending:false,isError:false,isSuccess:false})}))

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
        unitType: field('servicii'), paymentTerms: field('30 zile'), buyerCui: field('RO10000000'), periodType:field('FIXED_TERM'),serviceTerms:[] } },
  }
}
function review(document = fixture(), onEvidence = vi.fn()) {
  render(<MemoryRouter><ContractProposalReview document={document} onEvidence={onEvidence} onStale={vi.fn()} /></MemoryRouter>)
}
beforeEach(() => vi.clearAllMocks())

describe('Contract ingestion human review boundary', () => {
  it('shows the proposal without creating a contract automatically', () => {
    review()
    expect(screen.getByLabelText('Referință contract')).toHaveValue('AI-REFERENCE')
    expect(screen.getByText('Valorile editate sunt candidatele autoritative', { exact: false })).toBeInTheDocument()
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('submits edited values and the exact extraction revision only on explicit confirmation', async () => {
    const user = userEvent.setup(); const document = fixture(); review(document)
    await user.clear(screen.getByLabelText('Referință contract'))
    await user.type(screen.getByLabelText('Referință contract'), 'USER-CORRECTION')
    expect(handlers.confirm).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirmă contractul' }))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({ document, key: expect.any(String), contract: expect.objectContaining({ reference: 'USER-CORRECTION' }) }),expect.any(Object))
    expect(document.extraction?.proposal?.reference.value).toBe('AI-REFERENCE')
  })
  it('retains evidence page navigation independently of confirmation', async () => {
    const evidence = vi.fn(); review(fixture(), evidence)
    await userEvent.setup().click(screen.getAllByRole('button', { name: 'Pagina 1' })[0])
    expect(evidence).toHaveBeenCalledWith(1)
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('does not invent a missing currency and native validation blocks confirmation', async () => {
    const document = fixture(); document.extraction!.proposal!.currency = { value: null, status: 'MISSING', confidence: 'UNKNOWN', evidence: { page: null, snippet: '' }, alternatives: [] }
    review(document)
    expect(screen.getByLabelText('Monedă ISO')).toHaveValue('')
    expect(screen.getByText('Moneda ISO este obligatorie.')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Confirmă contractul' }))
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('blocks confirmation when authoritative unit type or payment terms are missing', () => {
    const document=fixture();document.extraction!.proposal!.unitType={...document.extraction!.proposal!.unitType,value:null,status:'MISSING'};document.extraction!.proposal!.paymentTerms={...document.extraction!.proposal!.paymentTerms,value:null,status:'MISSING'}
    review(document)
    expect(screen.getByRole('alert')).toHaveTextContent('Tipul unității / baza comercială este obligatoriu.')
    expect(screen.getByRole('alert')).toHaveTextContent('Termenii de plată sunt obligatorii.')
    expect(screen.getByRole('button',{name:'Confirmă contractul'})).toBeDisabled()
  })
  it('keeps incomplete service pricing empty for human review',()=>{
    const document=fixture();const missing={value:null,status:'MISSING' as const,confidence:'UNKNOWN' as const,evidence:{page:null,snippet:''},alternatives:[]}
    document.extraction!.proposal!.serviceTerms=[{serviceDescription:{...missing,value:'Contabilitate',status:'PRESENT',confidence:'HIGH',evidence:{page:2,snippet:'Contabilitate'}},pricingModel:missing,unitPrice:missing,currency:missing,unit:missing,quantitySource:missing,quantityValue:missing,quantityDriver:missing,billingFrequency:missing}]
    review(document)
    expect(screen.getByLabelText('Model tarifare 1')).toHaveValue('')
    expect(screen.getByLabelText('Preț 1')).toHaveValue('')
    expect(screen.getByText('Serviciul 1: modelul de tarifare este obligatoriu.')).toBeInTheDocument()
  })
  it('retains a clause without an expression as partial review, not an executable price rule',async()=>{
    const document=fixture();const extracted={value:'Tarif după grila de documente',status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:'Tarif după grila de documente'},alternatives:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'TIERED_PRICE'},narrative:extracted,evidence:{page:2,snippet:'Tarif după grila de documente'},confidence:'HIGH',rule:{id:'pending-grid',kind:'TIERED_PRICE',narrative:'Tarif după grila de documente',applicability:{},dateBasis:'INVOICE_ISSUE_DATE',expression:null,evidence:[{documentId:'document-1',page:2,snippet:'Tarif după grila de documente'}],blocking:true}}]
    review(document)
    expect(screen.getByRole('heading',{name:'Clauze extrase fără formulă executabilă'})).toBeInTheDocument()
    expect(screen.getByLabelText('Acoperire comercială')).toBeDisabled()
    await userEvent.setup().click(screen.getByRole('button',{name:'Confirmă contractul'}))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({contract:expect.objectContaining({coverage:'PARTIAL',commercialRules:[]})}),expect.any(Object))
  })
  it('requires explicit human confirmation for a normalized AI rule',async()=>{
    const document=fixture();const extracted={value:'Tarif fix de 500 RON',status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:'Tarif fix de 500 RON'},alternatives:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'FIXED_PRICE'},narrative:extracted,evidence:{page:2,snippet:'Tarif fix de 500 RON'},confidence:'HIGH',rule:{id:'fixed-500',kind:'FIXED_PRICE',narrative:'Tarif fix de 500 RON',applicability:{},dateBasis:'INVOICE_ISSUE_DATE',expression:{op:'literal',value:'500'},evidence:[{documentId:'document-1',page:2,snippet:'Tarif fix de 500 RON'}],blocking:true}}]
    const user=userEvent.setup();review(document)
    expect(screen.getByRole('heading',{name:'Reguli propuse de AI, neconfirmate'})).toBeInTheDocument()
    expect(screen.getByLabelText('Acoperire comercială')).toBeDisabled()
    await user.click(screen.getByRole('button',{name:'Confirmă contractul'}))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({contract:expect.objectContaining({coverage:'PARTIAL',commercialRules:[]})}),expect.any(Object))
    handlers.confirm.mockClear()
    await user.click(screen.getByRole('button',{name:'Confirmă regula'}))
    expect(screen.getByLabelText('Acoperire comercială')).toBeEnabled()
    await user.selectOptions(screen.getByLabelText('Acoperire comercială'),'COMPLETE')
    await user.click(screen.getByRole('button',{name:'Confirmă contractul'}))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({contract:expect.objectContaining({coverage:'COMPLETE',commercialRules:[expect.objectContaining({id:'fixed-500',expression:{op:'literal',value:'500'}})]})}),expect.any(Object))
  })
  it('allows an executable proposal to be confirmed after the contract was already confirmed',async()=>{
    const document=fixture();const extracted={value:'Tarif fix de 500 RON',status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:'Tarif fix de 500 RON'},alternatives:[]}
    document.status='CONFIRMED';document.confirmedContractId='contract-1';document.confirmedAt='2026-09-16T10:00:00Z';document.confirmedValues={supplierName:'Supplier SRL',supplierCui:'RO12345678',buyerCui:'RO10000000',reference:'AI-REFERENCE',effectiveFrom:'2026-01-01',effectiveTo:'2027-12-31',totalValue:'125000',currency:'RON',unitType:'servicii',paymentTerms:'30 zile',periodType:'FIXED_TERM',documentRole:'BASE_CONTRACT',relatedReference:'',coverage:'PARTIAL',serviceTerms:[],commercialRules:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'FIXED_PRICE'},narrative:extracted,evidence:{page:2,snippet:'Tarif fix de 500 RON'},confidence:'HIGH',rule:{id:'fixed-500',kind:'FIXED_PRICE',narrative:'Tarif fix de 500 RON',applicability:{},dateBasis:'INVOICE_ISSUE_DATE',expression:{op:'literal',value:'500'},evidence:[{documentId:'document-1',page:2,snippet:'Tarif fix de 500 RON'}],blocking:true}}]
    review(document)
    await userEvent.setup().click(screen.getByRole('button',{name:'Confirmă regula'}))
    expect(handlers.confirmRule).toHaveBeenCalledWith({ruleId:'fixed-500'})
  })
  it('allows a reviewer to complete a payment term without changing its source evidence',async()=>{
    const document=fixture();const extracted={value:'Plata în 5 zile de la remiterea facturii',status:'PRESENT' as const,confidence:'HIGH' as const,evidence:{page:2,snippet:'Plata în 5 zile de la remiterea facturii'},alternatives:[]}
    document.status='CONFIRMED';document.confirmedContractId='contract-1';document.confirmedAt='2026-09-16T10:00:00Z';document.confirmedValues={supplierName:'Supplier SRL',supplierCui:'RO12345678',buyerCui:'RO10000000',reference:'AI-REFERENCE',effectiveFrom:'2026-01-01',effectiveTo:'2027-12-31',totalValue:'125000',currency:'RON',unitType:'servicii',paymentTerms:'5 zile',periodType:'FIXED_TERM',documentRole:'BASE_CONTRACT',relatedReference:'',coverage:'PARTIAL',serviceTerms:[],commercialRules:[]}
    document.extraction!.proposal!.commercialClauses=[{kind:{...extracted,value:'PAYMENT_DUE'},narrative:extracted,evidence:{page:2,snippet:extracted.value},confidence:'HIGH',rule:{id:'payment-5',kind:'PAYMENT_DUE',narrative:extracted.value!,applicability:{},dateBasis:'',expression:null,evidence:[{documentId:'',page:2,snippet:extracted.value!}],blocking:true}}]
    review(document);const user=userEvent.setup();await user.type(screen.getByLabelText('Număr de zile payment-5'),'5');await user.click(screen.getByRole('button',{name:'Confirmă regula completată'}))
    expect(handlers.confirmRule).toHaveBeenCalledWith({ruleId:'payment-5',rule:expect.objectContaining({id:'payment-5',kind:'PAYMENT_DUE',dateBasis:'RECEIPT_DATE',expression:{op:'literal',value:'5',scale:2}})})
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
  it('blocks buyer mismatch rather than silently moving the document to another client', () => {
    const document=fixture();document.clientCui='RO10000000';document.extraction!.proposal!.buyerCui= {...document.extraction!.proposal!.buyerCui,value:'RO99999999'};review(document)
    expect(screen.getByRole('button', { name: 'Confirmă contractul' })).toBeDisabled()
    expect(screen.getByRole('alert')).toHaveTextContent('CUI-ul cumpărătorului')
  })
  it('recomputes buyer blocker from the edited reviewed value', async()=>{
    const document=fixture();document.clientCui='RO21592770';document.extraction!.proposal!.buyerCui={...document.extraction!.proposal!.buyerCui,value:'RO99999999'};review(document)
    const input=screen.getByLabelText('CUI cumpărător');await userEvent.setup().clear(input);await userEvent.setup().type(input,'ro 21592770')
    expect(screen.getByRole('button',{name:'Confirmă contractul'})).toBeEnabled()
  })
  it('models indefinite term without an end date',async()=>{
    review();await userEvent.setup().click(screen.getByLabelText('Nedeterminată'))
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
