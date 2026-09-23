import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RepositoryProvider } from '../../app/repository-context'
import { MockInvoiceRepository, mockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import type { ClassificationReviewItem } from '../../domain/invoice'
import type { AccountCatalogEntry } from '../../repositories/invoiceRepository'
import { AccountDecisionDialog } from './AccountDecisionDialog'

const base:ClassificationReviewItem={id:'decision',lineId:'line',lineLabel:'Serviciu generic',dimension:'ACCOUNT',proposedValue:'Necesită decizie',confidence:'Necesită revizuire',explanation:'Selectare manuală',legalBasis:'Confirmare contabilă',status:'PENDING',revision:1,modelVersion:'ACCOUNTING_DOMAIN_V2',source:'NO_MATCH',mappingScope:{clientDisplay:'Client generic',supplierDisplay:'Furnizor generic (RO123)',serviceIdentityKind:'SELLER_ITEM_ID',serviceIdentityValue:'SERVICE-1',normalizerVersion:'EXACT_IDENTIFIER_V1'}}

describe('AccountDecisionDialog',()=>{
  it('searches Diana accounts by code/name and creates reusable knowledge only explicitly',async()=>{
    const user=userEvent.setup();const submit=vi.fn()
    render(<RepositoryProvider repository={mockInvoiceRepository}><AccountDecisionDialog item={base} onCancel={()=>undefined} onSubmit={submit}/></RepositoryProvider>)
    expect(screen.queryByText(/Nu există conturi selectabile/)).not.toBeInTheDocument()
    await user.click(screen.getByRole('combobox',{name:'Cont contabil'}))
    await user.type(screen.getByLabelText('Caută în conturi'),'6281')
    const option=await screen.findByRole('option',{name:/6281 — Cheltuieli cu serviciile IT/})
    expect(option).not.toHaveTextContent(/uuid|mapping/i)
    await user.click(option)
    expect(screen.getByLabelText('Aplică doar acestei linii')).toBeChecked()
    expect(screen.queryByLabelText('Scope mapare reutilizabilă')).not.toBeInTheDocument()
    await user.click(screen.getByLabelText('Reutilizează pentru linii viitoare'))
    expect(screen.getByLabelText('Scope mapare reutilizabilă')).toHaveTextContent('Client generic')
    expect(screen.getByLabelText('Scope mapare reutilizabilă')).toHaveTextContent('SELLER_ITEM_ID: SERVICE-1')
    await user.click(screen.getByRole('button',{name:'Salvează decizia'}))
    expect(submit).toHaveBeenCalledWith(expect.objectContaining({code:'6281',name:'Cheltuieli cu serviciile IT'}),'CREATE','Decizie de cont confirmată de contabil.')
  })

  it('offers occurrence-only, correction and policy change for learned proposals and requires policy reason',async()=>{
    const user=userEvent.setup();const submit=vi.fn();const learned:ClassificationReviewItem={...base,source:'LEARNED_MAPPING',proposedValue:'6281',proposedTypedValue:{kind:'ACCOUNT',account:'6281'},mapping:{mappingId:'internal-uuid-not-for-display',version:2,revision:3,accountCode:'6281',serviceIdentityKind:'SELLER_ITEM_ID',serviceIdentityValue:'ITEM',normalizerVersion:'NORMALIZED_DESCRIPTION_V1'}}
    render(<RepositoryProvider repository={mockInvoiceRepository}><AccountDecisionDialog item={learned} onCancel={()=>undefined} onSubmit={submit}/></RepositoryProvider>)
    expect(screen.getByLabelText('Doar această apariție')).toBeInTheDocument()
    expect(screen.getByLabelText('Corectează maparea reutilizabilă')).toBeInTheDocument()
    expect(screen.queryByText('internal-uuid-not-for-display')).not.toBeInTheDocument()
    await user.click(screen.getByRole('combobox',{name:'Cont contabil'}))
    await user.type(screen.getByLabelText('Caută în conturi'),'6262')
    await waitFor(()=>expect(screen.getByRole('option',{name:/6262/})).toBeInTheDocument())
    await user.click(screen.getByRole('option',{name:/6262/}))
    await user.click(screen.getByLabelText('Schimbare de politică'))
    expect(screen.getByRole('button',{name:'Salvează decizia'})).toBeDisabled()
    await user.type(screen.getByLabelText(/Motiv/),'Politică aprobată')
    await user.click(screen.getByRole('button',{name:'Salvează decizia'}))
    expect(submit).toHaveBeenCalledWith(expect.objectContaining({code:'6262'}),'POLICY_CHANGE','Politică aprobată')
  })

  it('shows the account catalog loading error without allowing an arbitrary value',async()=>{
    class FailingAccountRepository extends MockInvoiceRepository {
      override async searchAccounts(_query:string):Promise<AccountCatalogEntry[]>{throw new Error('catalog unavailable')}
    }
    const user=userEvent.setup();const submit=vi.fn()
    render(<RepositoryProvider repository={new FailingAccountRepository()}><AccountDecisionDialog item={base} onCancel={()=>undefined} onSubmit={submit}/></RepositoryProvider>)
    await user.click(screen.getByRole('combobox',{name:'Cont contabil'}))
    expect(await screen.findByRole('alert')).toHaveTextContent('Catalogul de conturi nu a putut fi încărcat')
    expect(screen.getByRole('button',{name:'Salvează decizia'})).toBeDisabled()
    expect(submit).not.toHaveBeenCalled()
  })
})
