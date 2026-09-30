import type { CommercialRule, ContractDocument } from '../../repositories/invoiceRepository'
import { describeCommercialRule } from './commercialRuleText'

export function RuleExplanation({rule}:{rule:CommercialRule}){return <><p className="mt-2 rounded bg-[var(--surface)] p-2 text-sm">{describeCommercialRule(rule)}</p><RuleFormula rule={rule}/></>}
export function RuleFormula({rule}:{rule:CommercialRule}){return rule.expression?<details className="mt-1 text-xs text-[var(--text-muted)]"><summary className="cursor-pointer">Detalii tehnice (pentru suport)</summary><pre className="mt-1 overflow-x-auto whitespace-pre-wrap">{JSON.stringify(rule.expression,null,2)}</pre></details>:null}

export function ExtractionHistory({document}:{document:ContractDocument}){return <details className="mt-5 text-xs"><summary className="cursor-pointer font-semibold">Istoric și proveniență extragere</summary><p className="mt-3">Încărcat de {document.uploadedBy??'utilizator'} · {document.uploadedAt}</p>{(document.attempts??(document.extraction?[document.extraction]:[])).map(attempt=><div key={attempt.id} className="mt-3 break-all"><p>{attempt.model} · {attempt.schemaVersion} · {attempt.promptVersion}</p><p>{attempt.status} · {attempt.startedAt}{attempt.safeErrorCategory?` · ${attempt.safeErrorCategory}`:''}</p></div>)}</details>}
