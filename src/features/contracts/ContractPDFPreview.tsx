import { useCallback, useEffect, useRef, useState } from 'react'
import { getDocument, GlobalWorkerOptions, type PDFDocumentProxy } from 'pdfjs-dist'
import workerURL from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import { useInvoiceRepository } from '../../app/repository-context'
import { Button } from '../../components/ui/button'
import { findSnippets, spanRects, type HighlightRect, type PdfTextItem } from './pdfTextSearch'

GlobalWorkerOptions.workerSrc = workerURL

export interface HighlightResult { found: boolean; exact: boolean }

// Renders one page of a contract PDF. With `highlights`, the snippets are
// located in the page's text layer, marked over the canvas and scrolled into
// view; `locateAcrossPages` searches the other pages once when the snippets
// are not on the given page (evidence recorded without a page number).
// A page that already loaded the document passes it as `document`; the
// preview then renders from it and leaves its lifecycle to the owner.
export function ContractPDFPreview({clientId,documentId,page,onPage,highlights,locateAcrossPages=false,onHighlight,className='card sticky top-4 overflow-hidden',viewportClassName='max-h-[calc(100vh-12rem)]',document:shared,documentError='',onRetry}:{clientId:string;documentId:string;page:number;onPage:(page:number)=>void;highlights?:string[];locateAcrossPages?:boolean;onHighlight?:(result:HighlightResult)=>void;className?:string;viewportClassName?:string;document?:PDFDocumentProxy|null;documentError?:string;onRetry?:()=>void}) {
  const repository=useInvoiceRepository(); const canvas=useRef<HTMLCanvasElement>(null); const viewport=useRef<HTMLDivElement>(null); const pageFrame=useRef<HTMLDivElement>(null)
  const managed=shared!==undefined
  const [ownPDF,setPDF]=useState<PDFDocumentProxy|null>(null); const [ownError,setError]=useState(''); const [zoom,setZoom]=useState(1); const [fitWidth,setFitWidth]=useState(true); const [retry,setRetry]=useState(0); const [availableWidth,setAvailableWidth]=useState(0)
  const pdf=managed?shared:ownPDF; const error=(managed?documentError:'')||ownError
  const [rects,setRects]=useState<HighlightRect[]>([])
  const highlightKey=(highlights??[]).join('\u0000')
  const onPageRef=useRef(onPage); const onHighlightRef=useRef(onHighlight); const locatedKey=useRef('')
  useEffect(()=>{onPageRef.current=onPage;onHighlightRef.current=onHighlight})
  useEffect(()=>{const element=viewport.current;if(!element)return;const observer=new ResizeObserver(entries=>setAvailableWidth(entries[0]?.contentRect.width??0));observer.observe(element);return()=>observer.disconnect()},[])
  useEffect(()=>{if(managed)return;let cancelled=false;let task:ReturnType<typeof getDocument>|null=null;setPDF(null);setError('');const load=async()=>{if(repository.runtimeAuthority==='API'){const url=`/api/v1/clients/${encodeURIComponent(clientId)}/contract-documents/${encodeURIComponent(documentId)}/file`;task=getDocument({url,withCredentials:true,disableRange:false,disableStream:false})}else{const blob=await repository.getContractDocumentFile(clientId,documentId);if(blob.type&&blob.type!=='application/pdf')throw new Error('Răspunsul documentului nu este PDF.');task=getDocument({data:new Uint8Array(await blob.arrayBuffer())})}const loaded=await task.promise;if(!cancelled)setPDF(loaded)};void load().catch(reason=>{if(!cancelled)setError(reason instanceof Error?reason.message:'PDF indisponibil')});return()=>{cancelled=true;void task?.destroy()}},[clientId,documentId,repository,retry,managed])
  useEffect(()=>{
    let cancelled=false
    let renderTask:ReturnType<Awaited<ReturnType<PDFDocumentProxy['getPage']>>['render']>|undefined
    setRects([])
    const snippets=highlightKey?highlightKey.split('\u0000'):[]
    const textItems=async(pageNumber:number)=>(await (await pdf!.getPage(pageNumber)).getTextContent()).items.filter(item=>'str' in item) as PdfTextItem[]
    if(pdf&&canvas.current)void pdf.getPage(Math.min(Math.max(page,1),pdf.numPages)).then(async pdfPage=>{
      if(cancelled||!canvas.current)return
      const base=pdfPage.getViewport({scale:1});const target=fitWidth&&availableWidth?Math.max(.25,(availableWidth-32)/base.width):zoom;const scaled=pdfPage.getViewport({scale:target});const ratio=window.devicePixelRatio||1
      canvas.current.width=Math.floor(scaled.width*ratio);canvas.current.height=Math.floor(scaled.height*ratio);canvas.current.style.width=`${scaled.width}px`;canvas.current.style.height=`${scaled.height}px`
      renderTask=pdfPage.render({canvas:canvas.current,viewport:scaled,transform:ratio===1?undefined:[ratio,0,0,ratio,0,0]})
      await renderTask.promise
      if(cancelled||!snippets.length)return
      const items=(await pdfPage.getTextContent()).items.filter(item=>'str' in item) as PdfTextItem[]
      if(cancelled)return
      const matches=findSnippets(items,snippets)
      if(!matches.length&&locateAcrossPages&&locatedKey.current!==`${documentId}:${highlightKey}`){
        locatedKey.current=`${documentId}:${highlightKey}`
        for(let other=1;other<=pdf.numPages;other+=1){
          if(other===pdfPage.pageNumber)continue
          if(findSnippets(await textItems(other),snippets).length){if(!cancelled)onPageRef.current(other);return}
        }
      }
      setRects(matches.flatMap(match=>spanRects(items,match.spans,scaled.transform,scaled.scale)))
      onHighlightRef.current?.({found:matches.length>0,exact:matches.length>0&&matches.every(match=>match.exact)})
    }).catch(reason=>{if(!cancelled&&reason?.name!=='RenderingCancelledException')setError('Pagina PDF nu a putut fi randată.')})
    return()=>{cancelled=true;renderTask?.cancel()}
  },[pdf,page,zoom,fitWidth,availableWidth,highlightKey,documentId,locateAcrossPages])
  useEffect(()=>{
    const container=viewport.current;const frame=pageFrame.current
    if(!container||!frame||!rects.length)return
    const top=Math.min(...rects.map(rect=>rect.top))
    container.scrollTo({top:Math.max(0,frame.offsetTop+top-96),behavior:'smooth'})
  },[rects])
  const download=useCallback(async()=>{try{const blob=await repository.getContractDocumentFile(clientId,documentId);const url=URL.createObjectURL(blob);const anchor=window.document.createElement('a');anchor.href=url;anchor.download='contract.pdf';anchor.click();URL.revokeObjectURL(url)}catch{setError('Originalul nu a putut fi descărcat.')}},[clientId,documentId,repository])
  return <section className={className} aria-label="Previzualizare PDF contractual"><div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border)] p-3"><div className="flex gap-2"><Button variant="secondary" size="sm" disabled={!pdf||page<=1} onClick={()=>onPage(page-1)}>Anterior</Button><Button variant="secondary" size="sm" disabled={!pdf||page>=pdf.numPages} onClick={()=>onPage(page+1)}>Următor</Button></div><span className="text-xs">Pagina {page}{pdf?` / ${pdf.numPages}`:''}</span><div className="flex gap-2"><Button variant="secondary" size="sm" disabled={!pdf} aria-label="Micșorează" onClick={()=>{setFitWidth(false);setZoom(value=>Math.max(.35,value-.15))}}>−</Button><Button variant="secondary" size="sm" disabled={!pdf} aria-label="Mărește" onClick={()=>{setFitWidth(false);setZoom(value=>Math.min(3,value+.15))}}>+</Button><Button variant="secondary" size="sm" disabled={!pdf} onClick={()=>setFitWidth(true)}>Potrivește lățimea</Button></div></div><div ref={viewport} className={`relative overflow-auto bg-[var(--surface-subtle)] p-4 ${viewportClassName}`}>{error&&<div role="alert" className="space-y-3"><p>PDF-ul nu poate fi afișat. Originalul rămâne păstrat.</p><p className="text-xs text-[var(--text-muted)]">{error}</p><div className="flex gap-2"><Button type="button" variant="secondary" onClick={()=>{setError('');if(managed)onRetry?.();else setRetry(value=>value+1)}}>Reîncearcă</Button><Button type="button" variant="secondary" onClick={()=>void download()}>Descarcă originalul</Button></div></div>}{!error&&!pdf&&<p role="status">Se încarcă PDF-ul…</p>}<div ref={pageFrame} className="relative mx-auto w-fit"><canvas ref={canvas} className="block bg-white shadow-sm" aria-label={`Pagina PDF ${page}`}/>{rects.map((rect,index)=><span key={index} data-testid="pdf-highlight" aria-hidden="true" className="pointer-events-none absolute rounded-sm" style={{left:rect.left-2,top:rect.top-1,width:rect.width+4,height:rect.height+2,background:'rgba(250,204,21,.42)',boxShadow:'0 0 0 2px rgba(217,119,6,.85)',mixBlendMode:'multiply'}}/>)}</div></div></section>
}
