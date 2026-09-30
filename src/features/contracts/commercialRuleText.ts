import type { CommercialExpression, CommercialRule } from '../../repositories/invoiceRepository'

// Descrieri în limbaj natural pentru regulile comerciale. Textele urmăresc exact
// ce compară motorul backend (internal/commercialvalidation/engine.go); nu adaugă semantică nouă.

const kindLabels:Record<string,string>={
  IDENTITY:'Identitate furnizor',CONTRACT_REFERENCE:'Referință contract',FIXED_PRICE:'Preț fix',UNIT_RATE:'Tarif pe unitate',TIERED_PRICE:'Preț pe praguri',DISCOUNT:'Discount',TRANCHE:'Tranșă',PRORATA:'Prorata',MINIMUM:'Valoare minimă',MAXIMUM:'Valoare maximă',COST_PLUS:'Cost plus adaos',FX:'Curs valutar',VAT:'TVA',PAYMENT_DUE:'Termen de plată',FREQUENCY:'Periodicitate facturare',CREDIT_NOTE:'Storno',
}
export const ruleKindLabel=(kind?:string|null)=>kind?kindLabels[kind]??kind:'Clauză'

const dateBasisLabels:Record<string,string>={
  INVOICE_ISSUE_DATE:'data emiterii facturii',SERVICE_PERIOD_START:'începutul perioadei de servicii',SERVICE_PERIOD_END:'sfârșitul perioadei de servicii',RECEIPT_DATE:'primirea / remiterea facturii',ACCEPTANCE_DATE:'acceptarea serviciului',CONTRACT_ANNIVERSARY:'aniversarea contractului',
}
const dateBasisLabel=(basis?:string)=>dateBasisLabels[basis||'INVOICE_ISSUE_DATE']??basis

const frequencyLabels:Record<string,string>={MONTHLY:'lunară',QUARTERLY:'trimestrială',ANNUAL:'anuală',PER_OCCURRENCE:'per prestație'}

// Variabile calculate automat de motor din factură; restul se completează manual la verificarea facturii.
const automaticVariables:Record<string,string>={invoice_total:'totalul facturii',invoice_line_count:'numărul de linii din factură',invoice_quantity_total:'cantitatea totală facturată'}
const manualVariables:Record<string,string>={applicable_vat_rate:'cota TVA aplicabilă',tranche:'numărul tranșei'}
export function variableLabel(name:string){
  if(automaticVariables[name])return automaticVariables[name]
  if(manualVariables[name])return manualVariables[name]
  if(name.startsWith('unit_quantity_'))return 'cantitatea serviciului'
  return `valoarea „${name}”`
}

const unitPriceKinds=new Set(['FIXED_PRICE','UNIT_RATE','TIERED_PRICE','DISCOUNT','TRANCHE'])
const netValueKinds=new Set(['PRORATA','MINIMUM','MAXIMUM','COST_PLUS','FX'])

export function describeExpression(expr:CommercialExpression,nested=false):string{
  const args=expr.args??[]
  const wrap=(text:string)=>nested?`(${text})`:text
  const inner=(item:CommercialExpression)=>describeExpression(item,true)
  switch(expr.op){
    case 'literal':return expr.value??'—'
    case 'variable':return variableLabel(expr.variable??'')
    case 'add':return args.length===1?describeExpression(args[0],nested):wrap(args.map(inner).join(' + '))
    case 'multiply':return wrap(args.map(inner).join(' × '))
    case 'percent':return wrap(`${inner(args[0])}% din ${inner(args[1])}`)
    case 'min':return wrap(`cea mai mică valoare dintre ${args.map(inner).join(', ')}`)
    case 'max':return wrap(`cea mai mare valoare dintre ${args.map(inner).join(', ')}`)
    case 'prorate':return wrap(`${args.map(inner).join(' × ')} (prorata)`)
    case 'fx':return wrap(`${args.map(inner).join(' × ')} (conversie valutară)`)
    case 'round':return args[0]?describeExpression(args[0],nested):'—'
    case 'tier':{
      let previous:string|null|undefined
      const steps=(expr.tiers??[]).map(tier=>{const step=tier.upTo!=null?`până la ${tier.upTo} → ${tier.value}`:previous!=null?`peste ${previous} → ${tier.value}`:`orice valoare → ${tier.value}`;previous=tier.upTo;return step})
      return wrap(`grilă după ${args[0]?inner(args[0]):'—'}: ${steps.join('; ')}`)
    }
    default:return 'formulă nerecunoscută'
  }
}

function collectVariables(expr:CommercialExpression,into:Set<string>){
  if(expr.op==='variable'&&expr.variable)into.add(expr.variable)
  expr.args?.forEach(arg=>collectVariables(arg,into))
  return into
}

const isApplicableVAT=(expr:CommercialExpression)=>expr.op==='variable'&&expr.variable==='applicable_vat_rate'

export function describeCommercialRule(rule:Pick<CommercialRule,'kind'|'expression'|'currency'|'dateBasis'|'applicability'>):string{
  const expr=rule.expression
  if(rule.kind==='CREDIT_NOTE')return 'Facturile storno se verifică față de factura inițială.'
  if(rule.kind==='FREQUENCY'){const frequency=rule.applicability?.billingFrequency;return frequency?`Facturarea trebuie să fie ${frequencyLabels[frequency]??frequency}.`:'Periodicitatea facturării nu este precizată.'}
  if(!expr)return 'Clauza nu are încă o valoare verificabilă.'
  if(rule.kind==='CONTRACT_REFERENCE')return `Factura trebuie să menționeze contractul „${expr.value??''}”.`
  if(rule.kind==='IDENTITY')return `CUI-ul furnizorului de pe factură trebuie să fie ${expr.value??''}.`
  if(rule.kind==='VAT'&&isApplicableVAT(expr))return 'Se aplică cota TVA legală valabilă la data facturii, preluată automat din tabelul fiscal (Codul fiscal, art. 291).'
  const value=describeExpression(expr)
  const money=rule.currency?(expr.op==='literal'?`${value} ${rule.currency}`:`${value}, în ${rule.currency}`):value
  let sentence:string
  if(rule.kind==='VAT')sentence=`Cota TVA de pe factură trebuie să fie ${value}%.`
  else if(rule.kind==='PAYMENT_DUE')sentence=`Scadența facturii trebuie să fie la ${value} zile de la ${dateBasisLabel(rule.dateBasis)}.`
  else if(unitPriceKinds.has(rule.kind))sentence=`Prețul unitar de pe factură trebuie să fie ${money}.`
  else if(netValueKinds.has(rule.kind))sentence=`Valoarea netă a liniei de pe factură trebuie să fie ${money}.`
  else sentence=`Valoare contractuală: ${money}.`
  const manual=[...collectVariables(expr,new Set<string>())].filter(name=>!automaticVariables[name])
  return manual.length>0?`${sentence} Se completează la verificarea facturii: ${manual.map(variableLabel).join(', ')}.`:sentence
}
