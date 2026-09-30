import type { CommercialRule } from '../../repositories/invoiceRepository'
import { describeCommercialRule, describeExpression, ruleKindLabel } from './commercialRuleText'

const rule=(patch:Partial<CommercialRule>):CommercialRule=>({id:'r',kind:'FIXED_PRICE',narrative:'',applicability:{},dateBasis:'INVOICE_ISSUE_DATE',evidence:[],blocking:true,...patch})

describe('commercial rule plain-language description',()=>{
  it('describes the rules from a confirmed accounting services contract without technical notation',()=>{
    expect(describeCommercialRule(rule({kind:'FIXED_PRICE',currency:'RON',expression:{op:'literal',value:'500',scale:4}}))).toBe('Prețul unitar de pe factură trebuie să fie 500 RON.')
    expect(describeCommercialRule(rule({kind:'UNIT_RATE',currency:'RON',expression:{op:'literal',value:'50',scale:4}}))).toBe('Prețul unitar de pe factură trebuie să fie 50 RON.')
    expect(describeCommercialRule(rule({kind:'CONTRACT_REFERENCE',expression:{op:'literal',value:'102/25.06.2025',scale:4}}))).toBe('Factura trebuie să menționeze contractul „102/25.06.2025”.')
    expect(describeCommercialRule(rule({kind:'PAYMENT_DUE',dateBasis:'RECEIPT_DATE',expression:{op:'literal',value:'5',scale:2}}))).toBe('Scadența facturii trebuie să fie la 5 zile de la primirea / remiterea facturii.')
    expect(describeCommercialRule(rule({kind:'VAT',expression:{op:'variable',variable:'applicable_vat_rate'}}))).toBe('Se aplică cota TVA legală valabilă la data facturii, preluată automat din tabelul fiscal (Codul fiscal, art. 291).')
  })
  it('uses the invoice issue date when the payment basis is not specified, as the engine does',()=>{
    expect(describeCommercialRule(rule({kind:'PAYMENT_DUE',dateBasis:'',expression:{op:'literal',value:'30'}}))).toBe('Scadența facturii trebuie să fie la 30 zile de la data emiterii facturii.')
  })
  it('describes an explicit VAT percentage, supplier identity and net-value kinds',()=>{
    expect(describeCommercialRule(rule({kind:'VAT',expression:{op:'literal',value:'19'}}))).toBe('Cota TVA de pe factură trebuie să fie 19%.')
    expect(describeCommercialRule(rule({kind:'IDENTITY',expression:{op:'literal',value:'RO21592770'}}))).toBe('CUI-ul furnizorului de pe factură trebuie să fie RO21592770.')
    expect(describeCommercialRule(rule({kind:'MINIMUM',currency:'EUR',expression:{op:'literal',value:'1000'}}))).toBe('Valoarea netă a liniei de pe factură trebuie să fie 1000 EUR.')
  })
  it('names manual inputs that must be completed during invoice verification',()=>{
    expect(describeCommercialRule(rule({kind:'UNIT_RATE',currency:'RON',expression:{op:'multiply',args:[{op:'literal',value:'50'},{op:'variable',variable:'unit_quantity_salarizare'}]}}))).toBe('Prețul unitar de pe factură trebuie să fie 50 × cantitatea serviciului, în RON. Se completează la verificarea facturii: cantitatea serviciului.')
    expect(describeCommercialRule(rule({kind:'COST_PLUS',expression:{op:'percent',args:[{op:'literal',value:'10'},{op:'variable',variable:'invoice_total'}]}}))).toBe('Valoarea netă a liniei de pe factură trebuie să fie 10% din totalul facturii.')
  })
  it('describes nested and tiered formulas in words',()=>{
    expect(describeExpression({op:'min',args:[{op:'add',args:[{op:'literal',value:'100'},{op:'variable',variable:'invoice_total'}]},{op:'literal',value:'500'}]})).toBe('cea mai mică valoare dintre (100 + totalul facturii), 500')
    expect(describeExpression({op:'tier',args:[{op:'variable',variable:'invoice_line_count'}],tiers:[{upTo:'10',value:'300'},{upTo:'50',value:'250'},{upTo:null,value:'200'}]})).toBe('grilă după numărul de linii din factură: până la 10 → 300; până la 50 → 250; peste 50 → 200')
    expect(describeExpression({op:'round',args:[{op:'variable',variable:'custom_driver'}],scale:2})).toBe('valoarea „custom_driver”')
  })
  it('translates rule kinds and keeps unknown kinds visible',()=>{
    expect(ruleKindLabel('FIXED_PRICE')).toBe('Preț fix')
    expect(ruleKindLabel('PAYMENT_DUE')).toBe('Termen de plată')
    expect(ruleKindLabel('NEW_KIND')).toBe('NEW_KIND')
    expect(ruleKindLabel(null)).toBe('Clauză')
  })
})
