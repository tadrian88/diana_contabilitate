import * as Dialog from '@radix-ui/react-dialog'
import { AlertCircle, CheckCircle2, Download, FileCheck2, X } from 'lucide-react'
import { useState } from 'react'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { Invoice } from '../../domain/invoice'
import { useConfirmSagaImport, useDownloadSagaArtifact } from './invoice-hooks'

export function SagaExportCard({ invoice }: { invoice: Invoice }) {
  const download = useDownloadSagaArtifact(invoice)
  const confirmation = useConfirmSagaImport(invoice)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [note, setNote] = useState('')
  const exportView = invoice.sagaExport

  if (!exportView && invoice.sagaStatus !== 'FAILED') return null

  const saveArtifact = () => download.mutate(undefined, {
    onSuccess: ({ blob, filename }) => {
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = filename
      link.click()
      URL.revokeObjectURL(url)
    },
  })

  const confirm = () => confirmation.mutate(note || undefined, {
    onSuccess: () => {
      setDialogOpen(false)
      setNote('')
    },
  })

  const confirmed = invoice.pipelineStatus === 'EXPORTED' && exportView?.confirmationType === 'HUMAN'
  const failed = invoice.sagaStatus === 'FAILED' || exportView?.artifactStatus === 'FAILED'

  return <section aria-labelledby="saga-export-heading" className="card overflow-hidden">
    <div className="flex items-start justify-between gap-4 border-b border-[var(--border)] px-5 py-4">
      <div><div className="flex items-center gap-2"><FileCheck2 className="size-5 text-[var(--accent)]" /><h3 id="saga-export-heading" className="font-bold">Export SAGA</h3></div><p className="mt-1 text-xs text-[var(--text-muted)]">Predare manuală a fișierului pentru această factură</p></div>
      <Badge tone={failed ? 'danger' : confirmed ? 'success' : 'warning'}>{failed ? 'Generare eșuată' : confirmed ? 'Import confirmat manual' : 'Fișier pregătit'}</Badge>
    </div>
    <div className="space-y-4 p-5">
      {failed && <div className="flex items-start gap-2 rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"><AlertCircle className="mt-0.5 size-4 shrink-0" />Fișierul SAGA nu a putut fi generat. Confirmarea importului nu este disponibilă.</div>}
      {!failed && !confirmed && <>
        <p className="text-sm text-[var(--text-secondary)]">Descarcă XML-ul, importă-l manual în SAGA, apoi confirmă în Diana. Descărcarea singură nu marchează factura ca importată.</p>
        {exportView && <dl className="grid gap-3 text-sm sm:grid-cols-3"><Info label="Fișier" value={exportView.filename ?? 'XML SAGA'} /><Info label="Generat la" value={formatDateTime(exportView.generatedAt)} /><Info label="Ultima descărcare" value={exportView.downloadedAt ? formatDateTime(exportView.downloadedAt) : 'Nedescărcat din Diana'} /></dl>}
        <ol className="grid gap-2 text-sm sm:grid-cols-3"><Step number="1" text="Descarcă XML" /><Step number="2" text="Importă în SAGA" /><Step number="3" text="Confirmă importul" /></ol>
        <div className="flex flex-wrap gap-3">
          <Button variant="secondary" disabled={!exportView || exportView.artifactStatus !== 'GENERATED' || download.isPending} onClick={saveArtifact}><Download className="size-4" />{download.isPending ? 'Se descarcă…' : 'Descarcă XML'}</Button>
          <Button disabled={!exportView || exportView.artifactStatus !== 'GENERATED'} onClick={() => setDialogOpen(true)}>Marchează ca importat în SAGA</Button>
        </div>
      </>}
      {confirmed && <div className="flex items-start gap-3 rounded-lg border border-[var(--success-border)] bg-[var(--success-soft)] p-4"><CheckCircle2 className="mt-0.5 size-5 shrink-0 text-[var(--success)]" /><div><p className="text-sm font-bold">Import confirmat manual</p><p className="mt-1 text-xs text-[var(--text-secondary)]">{exportView.confirmedBy ?? 'Contabil'} · {exportView.confirmedAt ? formatDateTime(exportView.confirmedAt) : 'dată indisponibilă'}. Diana nu a primit o confirmare tehnică de la SAGA.</p></div></div>}
      {download.isError && <p role="alert" className="text-sm text-[var(--danger)]">Descărcarea nu a putut fi finalizată. Reîncarcă factura și încearcă din nou.</p>}
    </div>

    <Dialog.Root open={dialogOpen} onOpenChange={setDialogOpen}><Dialog.Portal><Dialog.Overlay className="fixed inset-0 z-40 bg-black/40" /><Dialog.Content className="dialog-content fixed left-1/2 top-1/2 z-50 w-[min(500px,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl"><Dialog.Title className="text-lg font-bold">Ai importat această factură în SAGA?</Dialog.Title><Dialog.Description className="mt-2 text-sm text-[var(--text-secondary)]">Diana nu poate verifica automat importul în SAGA. Confirmă doar după ce fișierul a fost importat.</Dialog.Description><label className="mt-5 block text-sm font-semibold">Notă opțională<textarea maxLength={500} value={note} onChange={(event) => setNote(event.target.value)} className="mt-2 min-h-20 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] p-3 font-normal outline-none focus:ring-2 focus:ring-[var(--focus)]" /></label>{confirmation.isError && <p role="alert" className="mt-3 rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">Operația SAGA nu a putut fi finalizată. Reîncarcă factura și încearcă din nou.</p>}<div className="mt-6 flex justify-end gap-3"><Dialog.Close asChild><Button variant="ghost">Anulează</Button></Dialog.Close><Button disabled={confirmation.isPending} onClick={confirm}>{confirmation.isPending ? 'Se confirmă…' : 'Confirmă importul'}</Button></div><Dialog.Close className="absolute right-4 top-4 rounded-md p-1 text-[var(--text-muted)]" aria-label="Închide"><X className="size-4" /></Dialog.Close></Dialog.Content></Dialog.Portal></Dialog.Root>
  </section>
}

function Info({ label, value }: { label: string; value: string }) { return <div className="rounded-lg bg-[var(--surface-subtle)] p-3"><dt className="text-xs text-[var(--text-muted)]">{label}</dt><dd className="mt-1 break-all font-semibold">{value}</dd></div> }
function Step({ number, text }: { number: string; text: string }) { return <li className="flex items-center gap-2 rounded-lg border border-[var(--border)] px-3 py-2"><span className="grid size-6 place-items-center rounded-full bg-[var(--info-soft)] text-xs font-bold text-[var(--info)]">{number}</span>{text}</li> }
function formatDateTime(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
