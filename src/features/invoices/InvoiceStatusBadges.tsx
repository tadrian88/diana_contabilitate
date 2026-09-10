import { AlertTriangle, CheckCircle2, CircleMinus, Clock3, Copy, LoaderCircle, XCircle } from 'lucide-react'
import { Badge } from '../../components/ui/badge'
import type { Invoice, PipelineStatus, SagaStatus } from '../../domain/invoice'
import { PIPELINE_LABELS } from '../../domain/invoice'
import { SAGA_LABELS, getAttentionStatus } from './invoice-view'

export function PipelineBadge({ status }: { status: PipelineStatus }) {
  const waiting = status.startsWith('AWAITING')
  const terminal = status === 'EXPORTED'
  const duplicate = status === 'DUPLICATE'
  const Icon = duplicate ? Copy : terminal ? CheckCircle2 : waiting ? Clock3 : status === 'EXPORTING' ? LoaderCircle : CircleMinus
  return <Badge tone={duplicate ? 'danger' : terminal ? 'success' : waiting ? 'warning' : 'info'}><Icon className={`size-3.5 ${status === 'EXPORTING' ? 'animate-spin' : ''}`} />{PIPELINE_LABELS[status]}</Badge>
}

export function SagaBadge({ status }: { status: SagaStatus }) {
  const Icon = status === 'EXPORTED' ? CheckCircle2 : status === 'FAILED' ? XCircle : status === 'EXPORTING' ? LoaderCircle : CircleMinus
  return <Badge tone={status === 'EXPORTED' ? 'success' : status === 'FAILED' ? 'danger' : status === 'NOT_READY' ? 'neutral' : 'info'}><Icon className={`size-3.5 ${status === 'EXPORTING' ? 'animate-spin' : ''}`} />{SAGA_LABELS[status]}</Badge>
}

export function AttentionBadge({ invoice }: { invoice: Invoice }) {
  const attention = getAttentionStatus(invoice)
  if (attention === 'OPEN') return <Badge tone="warning"><AlertTriangle className="size-3.5" />Acțiune necesară</Badge>
  if (attention === 'WAITING') return <Badge tone="info"><Clock3 className="size-3.5" />Condiție externă</Badge>
  return <Badge tone="success"><CheckCircle2 className="size-3.5" />Fără intervenție</Badge>
}

