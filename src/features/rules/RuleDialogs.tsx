import * as Dialog from '@radix-ui/react-dialog'
import { zodResolver } from '@hookform/resolvers/zod'
import { X } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Button } from '../../components/ui/button'
import type { ClassificationRule, Client, RuleVersion } from '../../domain/invoice'
import { useCreateClientOverride, useCreateRuleVersion } from './rule-hooks'

const schema = z.object({
  criteria: z.string().trim().min(5, 'Completează un criteriu demonstrativ.'),
  result: z.string().trim().min(3, 'Completează rezultatul demonstrativ.'),
  effectiveFrom: z.string().min(1, 'Selectează data de început.'),
  effectiveTo: z.string().optional(),
  clientId: z.string().optional(),
}).refine((value) => !value.effectiveTo || value.effectiveTo >= value.effectiveFrom, { path: ['effectiveTo'], message: 'Data finală trebuie să fie după data de început.' })
type RuleForm = z.infer<typeof schema>

const legalBasis = 'Exemplu demonstrativ — bază legală nevalidată'

export function NewVersionDialog({ rule, current, open, onOpenChange }: { rule: ClassificationRule; current: RuleVersion; open: boolean; onOpenChange: (open: boolean) => void }) {
  const mutation = useCreateRuleVersion(rule.id)
  const form = useForm<RuleForm>({ resolver: zodResolver(schema), defaultValues: defaults(current) })
  useEffect(() => { if (open) form.reset(defaults(current)) }, [current, form, open])
  return <RuleDialog title="Creează versiune nouă" description={`Versiunea ${current.version} rămâne păstrată în istoric. Domeniul ${rule.scope === 'GLOBAL' ? 'Regulă globală' : 'Override client'} nu se schimbă.`} open={open} onOpenChange={onOpenChange}>
    <RuleFields form={form} />
    <div className="mt-4 rounded-lg border border-[var(--warning-border)] bg-[var(--warning-soft)] p-3 text-xs text-[var(--warning)]">Bază legală: {legalBasis}</div>
    <div className="mt-6 flex justify-end gap-3"><Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>Anulează</Button><Button type="button" disabled={mutation.isPending} onClick={form.handleSubmit((value) => mutation.mutate(clean(value), { onSuccess: () => onOpenChange(false) }))}>Salvează versiunea nouă</Button></div>
  </RuleDialog>
}

export function ClientOverrideDialog({ rule, current, clients, open, onOpenChange, onCreated }: { rule: ClassificationRule; current: RuleVersion; clients: Client[]; open: boolean; onOpenChange: (open: boolean) => void; onCreated: () => void }) {
  const mutation = useCreateClientOverride(rule.id)
  const form = useForm<RuleForm>({ resolver: zodResolver(schema), defaultValues: { ...defaults(current), clientId: '' } })
  useEffect(() => { if (open) form.reset({ ...defaults(current), clientId: '' }) }, [current, form, open])
  return <RuleDialog title="Creează override pentru client" description="Regula globală rămâne neschimbată. Variația va fi explicit legată de clientul selectat." open={open} onOpenChange={onOpenChange}>
    <label className="block text-sm font-semibold">Client<select {...form.register('clientId')} className="mt-2 h-11 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]"><option value="">Selectează clientul</option>{clients.map((client) => <option key={client.id} value={client.id}>{client.name}</option>)}</select></label>{form.formState.errors.clientId && <p role="alert" className="mt-1 text-xs text-[var(--danger)]">{form.formState.errors.clientId.message}</p>}
    <div className="mt-4"><RuleFields form={form} /></div>
    <div className="mt-4 rounded-lg border border-[var(--warning-border)] bg-[var(--warning-soft)] p-3 text-xs text-[var(--warning)]">Bază legală: {legalBasis}</div>
    <div className="mt-6 flex justify-end gap-3"><Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>Anulează</Button><Button type="button" disabled={mutation.isPending} onClick={form.handleSubmit((value) => { if (!value.clientId) { form.setError('clientId', { message: 'Selectează clientul.' }); return } mutation.mutate({ ...clean(value), clientId: value.clientId }, { onSuccess: () => { onOpenChange(false); onCreated() } }) })}>Salvează override-ul</Button></div>
  </RuleDialog>
}

function RuleFields({ form }: { form: ReturnType<typeof useForm<RuleForm>> }) {
  return <div className="space-y-4">
    <Field label="Criteriu / pattern" error={form.formState.errors.criteria?.message}><textarea {...form.register('criteria')} rows={3} className="mt-2 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]" /></Field>
    <Field label="Rezultat clasificare" error={form.formState.errors.result?.message}><input {...form.register('result')} className="mt-2 h-11 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]" /></Field>
    <div className="grid grid-cols-2 gap-4"><Field label="Efectiv de la" error={form.formState.errors.effectiveFrom?.message}><input type="date" {...form.register('effectiveFrom')} className="mt-2 h-11 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]" /></Field><Field label="Efectiv până la" error={form.formState.errors.effectiveTo?.message}><input type="date" {...form.register('effectiveTo')} className="mt-2 h-11 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]" /></Field></div>
  </div>
}

function Field({ label, error, children }: { label: string; error?: string; children: React.ReactNode }) { return <label className="block text-sm font-semibold">{label}{children}{error && <span role="alert" className="mt-1 block text-xs text-[var(--danger)]">{error}</span>}</label> }
function RuleDialog({ title, description, open, onOpenChange, children }: { title: string; description: string; open: boolean; onOpenChange: (open: boolean) => void; children: React.ReactNode }) { return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal><Dialog.Overlay className="fixed inset-0 z-40 bg-black/40" /><Dialog.Content className="dialog-content fixed left-1/2 top-1/2 z-50 max-h-[90vh] w-[620px] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl"><Dialog.Title className="text-lg font-bold">{title}</Dialog.Title><Dialog.Description className="mt-1 text-sm text-[var(--text-secondary)]">{description}</Dialog.Description><div className="mt-5">{children}</div><Dialog.Close className="absolute right-4 top-4 rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-subtle)]" aria-label="Închide"><X className="size-4" /></Dialog.Close></Dialog.Content></Dialog.Portal></Dialog.Root> }
function defaults(current: RuleVersion): RuleForm { return { criteria: current.criteria, result: current.result, effectiveFrom: nextEffectiveDate(current.effectiveTo), effectiveTo: '' } }
function nextEffectiveDate(effectiveTo?: string) { if (!effectiveTo) return '2027-01-01'; const date = new Date(`${effectiveTo}T00:00:00.000Z`); date.setUTCDate(date.getUTCDate() + 1); return date.toISOString().slice(0, 10) }
function clean(value: RuleForm) { return { criteria: value.criteria.trim(), result: value.result.trim(), effectiveFrom: value.effectiveFrom, effectiveTo: value.effectiveTo || undefined } }
