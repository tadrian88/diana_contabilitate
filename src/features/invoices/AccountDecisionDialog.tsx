import * as Dialog from '@radix-ui/react-dialog'
import { useEffect, useState } from 'react'
import { ChevronDown, Search } from 'lucide-react'
import { Button } from '../../components/ui/button'
import { useInvoiceRepository } from '../../app/repository-context'
import type { AccountMappingAction, ClassificationReviewItem } from '../../domain/invoice'
import type { AccountCatalogEntry } from '../../repositories/invoiceRepository'

export function AccountDecisionDialog({item,onCancel,onSubmit}:{item:ClassificationReviewItem;onCancel:()=>void;onSubmit:(account:AccountCatalogEntry,action:AccountMappingAction,reason:string)=>void}) {
  const repository=useInvoiceRepository()
  const initialCode=item.mapping?.accountCode??item.proposedTypedValue?.account??''
  const [open,setOpen]=useState(false)
  const [query,setQuery]=useState('')
  const [results,setResults]=useState<AccountCatalogEntry[]>([])
  const [selected,setSelected]=useState<AccountCatalogEntry|null>(null)
  const [action,setAction]=useState<AccountMappingAction>('OCCURRENCE_ONLY')
  const [reason,setReason]=useState('')
  const [loading,setLoading]=useState(false)
  const [searchError,setSearchError]=useState(false)
  useEffect(()=>{if(!initialCode)return;let current=true;void repository.searchAccounts(initialCode).then(items=>{if(current)setSelected(items.find(account=>account.code===initialCode)??null)}).catch(()=>{if(current)setSearchError(true)});return()=>{current=false}},[initialCode,repository])
  useEffect(()=>{if(!open)return;let current=true;setLoading(true);setSearchError(false);const timer=setTimeout(()=>void repository.searchAccounts(query).then(items=>{if(current)setResults(items)}).catch(()=>{if(current){setResults([]);setSearchError(true)}}).finally(()=>current&&setLoading(false)),150);return()=>{current=false;clearTimeout(timer)}},[open,query,repository])
  const learned=item.source==='LEARNED_MAPPING'||Boolean(item.mapping)
  const actions:Array<{value:AccountMappingAction;label:string}> = learned
    ? [{value:'OCCURRENCE_ONLY',label:'Doar această apariție'},{value:'CORRECT',label:'Corectează maparea reutilizabilă'},{value:'POLICY_CHANGE',label:'Schimbare de politică'}]
    : item.source==='AMBIGUOUS' ? [{value:'OCCURRENCE_ONLY',label:'Aplică doar acestei linii'}] : [{value:'OCCURRENCE_ONLY',label:'Aplică doar acestei linii'},{value:'CREATE',label:'Reutilizează pentru linii viitoare'}]
  const reasonRequired=action==='POLICY_CHANGE'
  const actuallyChanged=!learned||selected?.code!==(item.proposedTypedValue?.account??item.mapping?.accountCode)
  return <Dialog.Root open onOpenChange={open=>!open&&onCancel()}><Dialog.Portal><Dialog.Overlay className="fixed inset-0 z-40 bg-black/40"/><Dialog.Content className="dialog-content fixed left-1/2 top-1/2 z-50 w-[min(95vw,38rem)] -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl">
    <Dialog.Title className="text-lg font-bold">{learned?'Schimbă decizia de cont':'Selectează contul Diana'}</Dialog.Title>
    <Dialog.Description className="mt-1 text-sm text-[var(--text-secondary)]">Caută după cod sau denumire. Sunt afișate numai conturile active și selectabile; selecția este validată din nou de backend.</Dialog.Description>
    <label className="mt-5 block text-sm font-semibold" id="account-select-label">Cont contabil</label>
    <div className="relative mt-2">
      <button type="button" role="combobox" aria-labelledby="account-select-label" aria-expanded={open} aria-controls="account-options" onClick={()=>setOpen(value=>!value)} className="flex h-11 w-full items-center justify-between rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-left text-sm">
        <span className={selected?'font-semibold':'text-[var(--text-muted)]'}>{selected?`${selected.code} — ${selected.name}`:'Selectează un cont'}</span><ChevronDown className={`size-4 transition-transform ${open?'rotate-180':''}`}/>
      </button>
      {open&&<div id="account-options" className="absolute z-20 mt-2 w-full overflow-hidden rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] shadow-xl">
        <div className="relative border-b border-[var(--border)] p-2"><Search className="absolute left-5 top-1/2 size-4 -translate-y-1/2 text-[var(--text-muted)]"/><input autoFocus aria-label="Caută în conturi" value={query} onChange={event=>setQuery(event.target.value)} placeholder="Caută după cod sau denumire" className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--surface)] pl-9 pr-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]"/></div>
        <div role="listbox" aria-label="Conturi Diana" className="max-h-56 overflow-auto">
          {loading&&<p className="p-3 text-sm text-[var(--text-muted)]">Se caută…</p>}
          {!loading&&searchError&&<p role="alert" className="p-3 text-sm text-[var(--danger)]">Catalogul de conturi nu a putut fi încărcat. Încearcă din nou.</p>}
          {!loading&&!searchError&&results.length===0&&<p className="p-3 text-sm text-[var(--text-muted)]">Nu există conturi selectabile pentru această căutare.</p>}
          {!loading&&results.map(account=><button type="button" role="option" aria-selected={selected?.code===account.code} key={account.code} onClick={()=>{setSelected(account);setOpen(false);setQuery('')}} className={`block w-full border-b border-[var(--border)] p-3 text-left text-sm last:border-0 hover:bg-[var(--surface-subtle)] ${selected?.code===account.code?'bg-[var(--accent-soft)]':''}`}><span className="font-semibold">{account.code} — {account.name}</span></button>)}
        </div>
      </div>}
    </div>
    <fieldset className="mt-5 space-y-2"><legend className="text-sm font-semibold">Efectul deciziei</legend>{actions.map(option=><label key={option.value} className="flex items-center gap-2 text-sm"><input type="radio" name="mapping-action" checked={action===option.value} onChange={()=>setAction(option.value)}/>{option.label}</label>)}</fieldset>
    {(action==='CREATE'||action==='CORRECT'||action==='POLICY_CHANGE')&&item.mappingScope&&<div className="mt-4 rounded-lg border border-[var(--border)] bg-[var(--surface-subtle)] p-3 text-sm" aria-label="Scope mapare reutilizabilă">
      <p className="font-semibold">Scope reutilizabil</p>
      <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs"><dt>Client</dt><dd>{item.mappingScope.clientDisplay}</dd><dt>Furnizor</dt><dd>{item.mappingScope.supplierDisplay}</dd><dt>Identitate</dt><dd>{item.mappingScope.serviceIdentityKind}: {item.mappingScope.serviceIdentityValue}</dd><dt>Normalizator</dt><dd>{item.mappingScope.normalizerVersion}</dd></dl>
    </div>}
    <label className="mt-5 block text-sm font-semibold" htmlFor="account-reason">Motiv {reasonRequired?'(obligatoriu)':'(opțional)'}</label>
    <textarea id="account-reason" value={reason} onChange={event=>setReason(event.target.value)} className="mt-2 min-h-20 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] p-3"/>
    {learned&&selected&&!actuallyChanged&&<p className="mt-2 text-xs text-[var(--text-muted)]">Selectează un cont diferit; pentru contul propus folosește acțiunea „Validează”.</p>}
    <div className="mt-6 flex justify-end gap-3"><Button type="button" variant="ghost" onClick={onCancel}>Anulează</Button><Button type="button" disabled={!selected||!actuallyChanged||(reasonRequired&&!reason.trim())} onClick={()=>selected&&onSubmit(selected,action,reason.trim()||'Decizie de cont confirmată de contabil.')}>Salvează decizia</Button></div>
  </Dialog.Content></Dialog.Portal></Dialog.Root>
}
