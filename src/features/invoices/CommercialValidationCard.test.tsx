import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { vi } from 'vitest'
import type { Invoice } from '../../domain/invoice'
import { CommercialValidationCard } from './CommercialValidationCard'

const handlers=vi.hoisted(()=>({aliases:vi.fn(),date:vi.fn(),variable:vi.fn(),resolve:vi.fn(),revoke:vi.fn()}))
const runState=vi.hoisted(()=>({activeSnapshotVersion:undefined as number|undefined}))
const vatEvidence=[{documentId:'doc-1',page:2,snippet:'4.3. Prețurile nu includ TVA. TVA se aplică în cota legală în vigoare.'}]
vi.mock('./invoice-hooks',()=>({
  useCommercialValidation:()=>({isLoading:false,data:{id:'run-1',invoiceId:'invoice-1',dossierId:'dossier-1',invoiceRevision:3,snapshotVersion:2,activeSnapshotVersion:runState.activeSnapshotVersion,engineVersion:'COMMERCIAL_VALIDATION_V2',outcome:'NEVERIFICABIL',createdAt:'2026-09-22',completedAt:'2026-09-22',findings:[
    {id:'finding-reference',ruleId:'reference',code:'CONTRACT_REFERENCE_MATCH',outcome:'CONFORM',actual:'102/25.06.2025',actualSource:'Descrierea liniei 1',expected:'102/25.06.2025',reason:'Referința facturii corespunde contractului asociat.',evidence:[{documentId:'doc-1',snippet:'102/25.06.2025'}]},
    {id:'vat-1',ruleId:'vat',lineId:'line-1',code:'VAT_RATE_MATCH',outcome:'CONFORM',actual:'21',expected:'21',calculation:'Cota legală la data facturii: 21% · Codul fiscal, art. 291',reason:'Cota TVA a liniei corespunde cotei legale la care trimite contractul.',evidence:vatEvidence},
    {id:'vat-2',ruleId:'vat',lineId:'line-2',code:'VAT_RATE_MATCH',outcome:'CONFORM',actual:'21',expected:'21',calculation:'Cota legală la data facturii: 21% · Codul fiscal, art. 291',reason:'Cota TVA a liniei corespunde cotei legale la care trimite contractul.',evidence:vatEvidence},
    {id:'price-2',ruleId:'service-payroll',lineId:'line-2',code:'PRICE_MATCH',outcome:'CONFORM',actual:'50.0000',expected:'50.0000',calculation:'Salarizare · 50.00 RON / salariat',reason:'Prețul liniei corespunde calculului contractual.',evidence:[{documentId:'doc-1',page:1,snippet:'50 lei/salariat'}]},
    {id:'finding-service',ruleId:'line-coverage',lineId:'line-1',code:'SERVICE_LINE_UNCOVERED',outcome:'NEVERIFICABIL',actual:'PRESTARI SERVICII CONTABILITATE',reason:'Linia facturii nu are o regulă comercială confirmată aplicabilă.',serviceCandidates:[{ruleId:'accounting-fee',label:'Contabilitate lunară',unit:'lună',score:0.86,sharedWords:['contabilitate'],lineWordCount:1,lineUnit:'MON',unitMatch:'COMPATIBLE',suggested:true,evidence:[{documentId:'doc-1',page:1,snippet:'500 lei'}]},{ruleId:'payroll-fee',label:'Salarizare',score:0,lineWordCount:1,lineUnit:'MON',unitMatch:'UNKNOWN'}]},
    {id:'finding-date',ruleId:'payment',code:'RULE_DATE_BASIS_MISSING',outcome:'NEVERIFICABIL',reason:'Lipsește data.',missingInputs:['remittance_date']},
    {id:'finding-vat-period',ruleId:'vat-manual',code:'RULE_INPUT_OUTSIDE_VALIDITY',outcome:'NEVERIFICABIL',actual:'21',actualSource:'act normativ',expected:'valabilă la 04.09.2026',calculation:'22.09.2026 – 22.09.2030',reason:'Valoarea este salvată, dar perioada nu acoperă factura.',missingInputs:['applicable_vat_rate']},
  ]}}),
  useResolveCommercialValidation:()=>({mutate:handlers.resolve,isPending:false,isError:false}),
  usePutCommercialVariable:()=>({mutate:handlers.variable,isPending:false,isSuccess:false,isError:false}),
  useConfirmCommercialServiceAliases:()=>({mutate:handlers.aliases,isPending:false,isError:false}),
  usePutCommercialDateFact:()=>({mutate:handlers.date,isPending:false,isSuccess:false,isError:false}),
}))
vi.mock('../contracts/contract-hooks',()=>({
  useCommercialServiceAliases:()=>({data:[]}),
  useRevokeCommercialServiceAlias:()=>({mutate:handlers.revoke,isPending:false,isError:false}),
}))
vi.mock('../contracts/ContractPDFPreview',()=>({
  ContractPDFPreview:({documentId,page,highlights}:{documentId:string;page:number;highlights?:string[]})=><div data-testid="contract-pdf">{documentId} · pagina {page} · {(highlights??[]).join(' | ')}</div>,
}))

const line=(id:string,position:number,description:string,amount:number)=>({id,position,description,unit:'MON',quantity:1,unitPrice:{amount,currency:'RON'},netValue:{amount,currency:'RON'},vatValue:{amount:amount*0.21,currency:'RON'},grossValue:{amount:amount*1.21,currency:'RON'},vatLabel:'21%',classifications:[]})
const invoice={id:'invoice-1',clientId:'client-1',supplierName:'FUTURE CONTA S.R.L.',supplierCui:'RO21592770',documentNumber:'FCO nr. 0878',issueDate:'2026-09-04',total:{amount:665.5,currency:'RON'},spvReference:'SPV-1',pipelineStatus:'AWAITING_COMMERCIAL_REVIEW',sagaStatus:'NOT_READY',revision:3,activity:[],lines:[line('line-1',1,'PRESTARI SERVICII CONTABILITATE',500),line('line-2',2,'Salarizare',50)]} as unknown as Invoice

const renderCard=()=>render(<MemoryRouter><CommercialValidationCard invoice={invoice}/></MemoryRouter>)
beforeEach(()=>{vi.clearAllMocks();runState.activeSnapshotVersion=undefined})

it('groups checks by invoice and line and states the VAT rate once, as VAT',()=>{
  renderCard()
  expect(screen.getAllByText('Cota TVA')).toHaveLength(1)
  expect(screen.getByText('· liniile 1–2')).toBeInTheDocument()
  expect(screen.queryByText('Prețul este corect')).not.toBeInTheDocument()
  expect(screen.getByText('Referința contractului')).toBeInTheDocument()
  expect(screen.getByText('Descrierea liniei 1')).toBeInTheDocument()
  expect(screen.getByText('Sugestie: Contabilitate lunară')).toBeInTheDocument()
  expect(screen.getByText('Serviciu contractual: „Salarizare”')).toBeInTheDocument()
  expect(screen.getByText('3 de rezolvat')).toBeInTheDocument()
})

it('maps lines to services in one step, with the suggestion preselected and reuse on by default',async()=>{
  const user=userEvent.setup()
  renderCard()
  await user.click(screen.getByRole('button',{name:'Asociază serviciile (1)'}))
  const dialog=screen.getByRole('dialog',{name:'Asociază liniile facturii cu serviciile din contract'})
  expect(within(dialog).getByRole('radio',{name:/Contabilitate lunară/})).toBeChecked()
  expect(within(dialog).getByText('UM compatibilă: MON = lună')).toBeInTheDocument()
  expect(within(dialog).getByLabelText('Ține minte pentru facturile viitoare din acest contract')).toBeChecked()
  expect(await within(dialog).findByTestId('contract-pdf')).toHaveTextContent('doc-1 · pagina 1 · 500 lei')
  await user.click(within(dialog).getByRole('button',{name:'Confirmă asocierea și revalidează'}))
  expect(handlers.aliases).toHaveBeenCalledWith({runId:'run-1',expectedInvoiceRevision:3,reuseForDossier:true,choices:[{lineId:'line-1',serviceId:'accounting-fee'}]},expect.anything())
})

it('lets the reviewer pick another service or none, never by price',async()=>{
  const user=userEvent.setup()
  renderCard()
  await user.click(screen.getByRole('button',{name:'Asociază serviciile (1)'}))
  const dialog=screen.getByRole('dialog')
  expect(within(dialog).getByText(/Prețul nu contează aici/)).toBeInTheDocument()
  await user.click(within(dialog).getByRole('radio',{name:/Serviciul nu apare în contract/}))
  const outcomes=within(dialog).getByRole('group',{name:'Ce faci cu linia care nu apare în contract'})
  expect(within(outcomes).getByRole('link',{name:'Încarcă anexa cu acest serviciu'})).toHaveAttribute('href',expect.stringContaining('/contracts/upload?clientId='))
  expect(within(outcomes).getByRole('button',{name:'Acceptă excepția'})).toBeDisabled()
  expect(within(dialog).queryByRole('button',{name:/Confirmă asocierea/})).not.toBeInTheDocument()
  await user.click(within(outcomes).getByRole('button',{name:'Cere corecția facturii'}))
  expect(handlers.resolve).toHaveBeenCalledWith(expect.objectContaining({findingId:'finding-service',action:'WAIT_FOR_CORRECTION'}),expect.anything())
  await user.click(within(dialog).getByRole('radio',{name:/^Salarizare/}))
  await user.click(within(dialog).getByRole('button',{name:'Confirmă asocierea și revalidează'}))
  expect(handlers.aliases).toHaveBeenCalledWith(expect.objectContaining({choices:[{lineId:'line-1',serviceId:'payroll-fee'}]}),expect.anything())
})

it('shows the invoice and the cited contract text side by side and walks through the checks',async()=>{
  const user=userEvent.setup()
  renderCard()
  await user.click(screen.getByRole('button',{name:'Verifică Cota TVA · liniile 1–2'}))
  const dialog=screen.getByRole('dialog',{name:'Cota TVA · liniile 1–2'})
  expect(within(dialog).getAllByTestId('invoice-highlight')).toHaveLength(2)
  expect(await within(dialog).findByTestId('contract-pdf')).toHaveTextContent('doc-1 · pagina 2 · 4.3. Prețurile nu includ TVA.')
  expect(within(dialog).getByText(/Codul fiscal, art. 291/)).toBeInTheDocument()
  await user.click(within(dialog).getByRole('button',{name:'Verificarea următoare'}))
  expect(screen.getByRole('dialog',{name:'Preț unitar · Linia 2'})).toBeInTheDocument()
  expect(within(screen.getByRole('dialog')).getByText('/Invoice/InvoiceLine[2]/cac:Price/cbc:PriceAmount')).toBeInTheDocument()
})

it('asks for the invoice-specific remittance date and its evidence',async()=>{
  renderCard()
  const user=userEvent.setup()
  await user.type(screen.getByLabelText('Data remiterii facturii'),'2026-09-04')
  await user.type(screen.getByPlaceholderText('Ex.: confirmare SPV sau mesaj e-mail'),'Confirmare de transmitere SPV')
  await user.click(screen.getByRole('button',{name:'Salvează data cu dovada'}))
  expect(handlers.date).toHaveBeenCalledWith({kind:'REMITTANCE',date:'2026-09-04',sourceReference:'Confirmare de transmitere SPV'})
})

it('shows a saved VAT value whose validity misses the invoice and prevents another invalid period',async()=>{
  renderCard()
  expect(screen.getByText('Informația este salvată, dar nu este valabilă la data facturii')).toBeInTheDocument()
  expect(screen.getByLabelText('Valoare applicable_vat_rate')).toHaveValue('21')
  expect(screen.getByLabelText('Sursă applicable_vat_rate')).toHaveValue('act normativ')
  expect(screen.getByText(/Perioada salvată: 22\.09\.2026 – 22\.09\.2030/)).toBeInTheDocument()
  expect(screen.getByText('Precizează data reală de la care această cotă TVA este valabilă.')).toBeInTheDocument()
  const dateInputs=screen.getByLabelText('Valoare applicable_vat_rate').closest('form')!.querySelectorAll('input[type="date"]')
  await userEvent.setup().clear(dateInputs[0] as HTMLInputElement)
  await userEvent.setup().type(dateInputs[0] as HTMLInputElement,'2026-09-22')
  expect(screen.getByText('Perioada trebuie să includă data facturii: 04.09.2026.')).toBeInTheDocument()
  expect(screen.getByRole('button',{name:'Salvează cu proveniență'})).toBeDisabled()
})

it('asks to run again when the contract changed after the result shown',async()=>{
  runState.activeSnapshotVersion=3
  const user=userEvent.setup()
  renderCard()
  expect(screen.getByText(/Contractul are acum versiunea 3; rezultatul de mai jos a folosit versiunea 2/)).toBeInTheDocument()
  await user.click(screen.getByRole('button',{name:'Rulează din nou'}))
  expect(handlers.resolve).toHaveBeenCalledWith(expect.objectContaining({runId:'run-1',action:'RERUN'}))
})

it('does not ask to run again while the result matches the contract version',()=>{
  runState.activeSnapshotVersion=2
  renderCard()
  expect(screen.queryByText(/Contractul are acum versiunea/)).not.toBeInTheDocument()
})
