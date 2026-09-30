import { useCallback, useEffect, useMemo, useState } from 'react'
import type { PDFDocumentProxy } from 'pdfjs-dist'
import { useInvoiceRepository } from '../../app/repository-context'
import { findSnippets, type PdfTextItem } from './pdfTextSearch'

export type SnippetLocation = 'EXACT' | 'PARTIAL' | 'NONE'
export interface LocatedSnippets { location: SnippetLocation; page?: number }

// Where a set of snippets appears in the document: on the cited page first,
// otherwise on any page. EXACT means every snippet was found whole on one
// page; PARTIAL that only some were, or only as the start of a wrapped cell.
export function locateSnippets(pages: PdfTextItem[][], snippets: string[], page?: number | null): LocatedSnippets {
  const wanted = [...new Set(snippets.map((snippet) => snippet.trim()).filter(Boolean))]
  if (!wanted.length) return { location: 'NONE' }
  const numbers = pages.map((_, index) => index + 1)
  const order = page && page >= 1 && page <= pages.length ? [page, ...numbers.filter((number) => number !== page)] : numbers
  let partial: LocatedSnippets | undefined
  for (const number of order) {
    const matches = findSnippets(pages[number - 1], wanted)
    if (!matches.length) continue
    if (matches.length === wanted.length && matches.every((match) => match.exact)) return { location: 'EXACT', page: number }
    partial ??= { location: 'PARTIAL', page: number }
  }
  return partial ?? { location: 'NONE' }
}

export interface ContractPdf {
  pdf: PDFDocumentProxy | null
  error: string
  retry: () => void
  // The text of every page has been read, so locate() answers for good.
  ready: boolean
  // No page has a text layer (a scan): nothing can be located.
  scanned: boolean
  locate: (snippets: string[], page?: number | null) => LocatedSnippets
}

type LoadingTask = ReturnType<(typeof import('pdfjs-dist'))['getDocument']>

// Loads a contract PDF once for the review page: the viewer renders from it
// and every extracted value is looked up in its text. pdf.js is imported
// inside the effect, so nothing loads it before the page needs the document.
export function useContractPdf(clientId: string, documentId: string): ContractPdf {
  const repository = useInvoiceRepository()
  const [pdf, setPdf] = useState<PDFDocumentProxy | null>(null)
  const [pages, setPages] = useState<PdfTextItem[][] | null>(null)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let cancelled = false
    let task: LoadingTask | null = null
    setPdf(null)
    setPages(null)
    setError('')
    const load = async () => {
      const [pdfjs, worker] = await Promise.all([import('pdfjs-dist'), import('pdfjs-dist/build/pdf.worker.min.mjs?url')])
      pdfjs.GlobalWorkerOptions.workerSrc = worker.default
      if (repository.runtimeAuthority === 'API') {
        task = pdfjs.getDocument({ url: `/api/v1/clients/${encodeURIComponent(clientId)}/contract-documents/${encodeURIComponent(documentId)}/file`, withCredentials: true, disableRange: false, disableStream: false })
      } else {
        const blob = await repository.getContractDocumentFile(clientId, documentId)
        if (blob.type && blob.type !== 'application/pdf') throw new Error('Răspunsul documentului nu este PDF.')
        task = pdfjs.getDocument({ data: new Uint8Array(await blob.arrayBuffer()) })
      }
      const loaded = await task.promise
      if (cancelled) return
      setPdf(loaded)
      const text: PdfTextItem[][] = []
      for (let number = 1; number <= loaded.numPages; number += 1) {
        const content = await (await loaded.getPage(number)).getTextContent()
        if (cancelled) return
        text.push(content.items.filter((item) => 'str' in item) as PdfTextItem[])
      }
      setPages(text)
    }
    void load().catch((reason) => { if (!cancelled) setError(reason instanceof Error ? reason.message : 'PDF indisponibil') })
    return () => { cancelled = true; void task?.destroy() }
  }, [clientId, documentId, repository, attempt])
  const retry = useCallback(() => setAttempt((value) => value + 1), [])
  const scanned = useMemo(() => Boolean(pages) && pages!.every((items) => items.every((item) => !item.str.trim())), [pages])
  const locate = useCallback((snippets: string[], page?: number | null) => pages ? locateSnippets(pages, snippets, page) : { location: 'NONE' as const }, [pages])
  return { pdf, error, retry, ready: Boolean(pages) || Boolean(error), scanned, locate }
}
