import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { vi } from 'vitest'
import type { Invoice } from '../../domain/invoice'
import { CommercialValidationCard } from './CommercialValidationCard'

const handlers=vi.hoisted(()=>({alias:vi.fn(),date:vi.fn(),variable:vi.fn(),resolve:vi.fn()}))
vi.mock('./invoice-hooks',()=>({
  useCommercialValidation:()=>({isLoading:false,data:{id:'run-1',invoiceId:'invoice-1',dossierId:'dossier-1',invoiceRevision:3,snapshotVersion:2,engineVersion:'COMMERCIAL_VALIDATION_V1',outcome:'NEVERIFICABIL',createdAt:'2026-09-22',completedAt:'2026-09-22',findings:[{id:'finding-reference',ruleId:'reference',code:'CONTRACT_REFERENCE_MATCH',outcome:'CONFORM',actual:'102/25.06.2025',actualSource:'Descrierea liniei 1',expected:'102/25.06.2025',reason:'Referința facturii corespunde contractului asociat.',evidence:[{documentId:'doc-1',snippet:'102/25.06.2025'}]},{id:'finding-coverage',ruleId:'coverage',code:'CONTRACT_COVERAGE_INCOMPLETE',outcome:'NEVERIFICABIL',reason:'Dosarul nu are acoperire.'},{id:'finding-service',ruleId:'line-coverage',lineId:'line-1',code:'SERVICE_LINE_UNCOVERED',outcome:'NEVERIFICABIL',actual:'PRESTARI SERVICII CF. CTR.',reason:'Confirmă formularea.',serviceCandidates:[{ruleId:'accounting-fee',label:'Contabilitate 500 RON'},{ruleId:'payroll-fee',label:'Salarizare 50 RON'}]},{id:'finding-date',ruleId:'payment',code:'RULE_DATE_BASIS_MISSING',outcome:'NEVERIFICABIL',reason:'Lipsește data.',missingInputs:['remittance_date']},{id:'finding-vat-period',ruleId:'vat',code:'RULE_INPUT_OUTSIDE_VALIDITY',outcome:'NEVERIFICABIL',actual:'21',actualSource:'act normativ',expected:'valabilă la 04.09.2026',calculation:'22.09.2026 – 22.09.2030',reason:'Valoarea este salvată, dar perioada nu acoperă factura.',missingInputs:['applicable_vat_rate']}]}}),
  useResolveCommercialValidation:()=>({mutate:handlers.resolve,isPending:false}),
  usePutCommercialVariable:()=>({mutate:handlers.variable,isPending:false,isSuccess:false,isError:false}),
  useConfirmCommercialServiceAlias:()=>({mutate:handlers.alias,isPending:false,isSuccess:false,isError:false}),
  usePutCommercialDateFact:()=>({mutate:handlers.date,isPending:false,isSuccess:false,isError:false}),
}))

const invoice={id:'invoice-1',clientId:'client-1',supplierName:'FUTURE CONTA S.R.L.',supplierCui:'RO21592770',documentNumber:'FCO nr. 0878',issueDate:'2026-09-04',total:{amount:605,currency:'RON'},spvReference:'SPV-1',pipelineStatus:'AWAITING_COMMERCIAL_REVIEW',sagaStatus:'NOT_READY',revision:3,activity:[],lines:[{id:'line-1',position:1,description:'PRESTARI SERVICII CF. CTR.',unit:'H87',quantity:1,unitPrice:{amount:500,currency:'RON'},netValue:{amount:500,currency:'RON'},vatValue:{amount:105,currency:'RON'},grossValue:{amount:605,currency:'RON'},vatLabel:'21%',classifications:[]}]} as unknown as Invoice

beforeEach(()=>vi.clearAllMocks())

it('shows business labels, invoice provenance and one-time service association',async()=>{
  render(<MemoryRouter><CommercialValidationCard invoice={invoice}/></MemoryRouter>)
  expect(screen.getByText('Referința contractului este corectă')).toBeInTheDocument()
  expect(screen.getByText('Sursă în factură: Descrierea liniei 1')).toBeInTheDocument()
  expect(screen.getByText('Asociază linia facturii cu serviciul contractual')).toBeInTheDocument()
  expect(screen.queryByRole('button',{name:'Acceptă excepția'})).not.toBeInTheDocument()
  await userEvent.setup().selectOptions(screen.getByLabelText('Serviciul contractual pentru linia 1'),'accounting-fee')
  await userEvent.setup().click(screen.getByRole('button',{name:'Confirmă asocierea'}))
  expect(handlers.alias).toHaveBeenCalledWith({serviceId:'accounting-fee',lineId:'line-1',reuseForDossier:false})
})

it('does not ask for an invoice exception while contractual service selection is available',()=>{
  render(<MemoryRouter><CommercialValidationCard invoice={invoice}/></MemoryRouter>)
  expect(screen.getByRole('option',{name:'Contabilitate 500 RON'})).toBeInTheDocument()
  expect(screen.queryByRole('button',{name:'Acceptă excepția'})).not.toBeInTheDocument()
})

it('asks for the invoice-specific remittance date and its evidence',async()=>{
  render(<MemoryRouter><CommercialValidationCard invoice={invoice}/></MemoryRouter>)
  const user=userEvent.setup()
  await user.type(screen.getByLabelText('Data remiterii facturii'),'2026-09-04')
  await user.type(screen.getByPlaceholderText('Ex.: confirmare SPV sau mesaj e-mail'),'Confirmare de transmitere SPV')
  await user.click(screen.getByRole('button',{name:'Salvează data cu dovada'}))
  expect(handlers.date).toHaveBeenCalledWith({kind:'REMITTANCE',date:'2026-09-04',sourceReference:'Confirmare de transmitere SPV'})
})

it('shows a saved VAT value whose validity misses the invoice and prevents another invalid period',async()=>{
  render(<MemoryRouter><CommercialValidationCard invoice={invoice}/></MemoryRouter>)
  expect(screen.getByText('Informația este salvată, dar nu este valabilă la data facturii')).toBeInTheDocument()
  expect(screen.getByLabelText('Valoare applicable_vat_rate')).toHaveValue('21')
  expect(screen.getByLabelText('Sursă applicable_vat_rate')).toHaveValue('act normativ')
  expect(screen.getByText('Perioada salvată: 22.09.2026 – 22.09.2030')).toBeInTheDocument()
  expect(screen.getByText('Precizează data reală de la care această cotă TVA este valabilă.')).toBeInTheDocument()
  const dateInputs=screen.getByLabelText('Valoare applicable_vat_rate').closest('form')!.querySelectorAll('input[type="date"]')
  await userEvent.setup().clear(dateInputs[0] as HTMLInputElement)
  await userEvent.setup().type(dateInputs[0] as HTMLInputElement,'2026-09-22')
  expect(screen.getByText('Perioada trebuie să includă data facturii: 04.09.2026.')).toBeInTheDocument()
  expect(screen.getByRole('button',{name:'Salvează cu proveniență'})).toBeDisabled()
})
