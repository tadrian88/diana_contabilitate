export function LegalSourceLink({ url }: { url: string }) {
 let safe = false
 try { const value = new URL(url); safe = value.protocol === 'https:' && !value.username && !value.password && ['legislatie.just.ro', 'static.anaf.ro', 'www.anaf.ro', 'mfinante.gov.ro', 'www.mfinante.gov.ro'].includes(value.hostname) } catch { /* Invalid provenance link remains plain text. */ }
 return safe ? <a href={url} target="_blank" rel="noopener noreferrer" className="text-[var(--accent)] underline">Consultă sursa oficială externă</a> : <span>Sursă externă indisponibilă</span>
}
