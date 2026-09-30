// Locates evidence snippets in the text layer of a PDF page so the viewer can
// highlight the exact words a finding relies on. Matching ignores case,
// Romanian diacritic variants (ş/ș, ţ/ț), dash styles and all whitespace,
// because PDF text items split words and lines unpredictably.

export interface PdfTextItem { str: string; transform: number[]; width: number; height: number }
// A span of one text item, by character offsets into item.str.
export interface PdfTextSpan { itemIndex: number; start: number; end: number }
export interface PdfSnippetMatch { snippet: string; spans: PdfTextSpan[]; exact: boolean }

const folds: Record<string, string> = { 'ă': 'a', 'â': 'a', 'î': 'i', 'ș': 's', 'ş': 's', 'ț': 't', 'ţ': 't', '—': '-', '–': '-', '‐': '-', '‑': '-', '’': "'", '„': '"', '”': '"', '“': '"' }

function foldChar(char: string) {
  const lower = char.toLocaleLowerCase('ro-RO')
  return folds[lower] ?? lower
}

export function normalizeForSearch(value: string) {
  return Array.from(value).filter((char) => !/\s/.test(char)).map(foldChar).join('')
}

interface IndexedText { text: string; origin: Array<{ itemIndex: number; charIndex: number }> }

function indexItems(items: PdfTextItem[]): IndexedText {
  let text = ''
  const origin: IndexedText['origin'] = []
  items.forEach((item, itemIndex) => {
    Array.from(item.str).forEach((char, charIndex) => {
      if (/\s/.test(char)) return
      text += foldChar(char)
      origin.push({ itemIndex, charIndex })
    })
  })
  return { text, origin }
}

function occurrences(haystack: string, needle: string) {
  const result: number[] = []
  if (!needle) return result
  for (let index = haystack.indexOf(needle); index >= 0; index = haystack.indexOf(needle, index + 1)) result.push(index)
  return result
}

function spansFor(indexed: IndexedText, start: number, length: number): PdfTextSpan[] {
  const byItem = new Map<number, PdfTextSpan>()
  for (let position = start; position < start + length; position += 1) {
    const { itemIndex, charIndex } = indexed.origin[position]
    const span = byItem.get(itemIndex)
    if (span) span.end = Math.max(span.end, charIndex + 1)
    else byItem.set(itemIndex, { itemIndex, start: charIndex, end: charIndex + 1 })
  }
  return [...byItem.values()]
}

// Candidate needles for one snippet: the whole snippet, then ever shorter word
// prefixes (never below three words or 60% of it), because a table cell that
// wraps puts other cells between its lines in the text stream.
function needles(snippet: string) {
  const words = snippet.trim().split(/\s+/).filter(Boolean)
  const result = [{ needle: normalizeForSearch(snippet), exact: true }]
  const minimum = Math.max(3, Math.ceil(words.length * 0.6))
  for (let count = words.length - 1; count >= minimum; count -= 1) result.push({ needle: normalizeForSearch(words.slice(0, count).join(' ')), exact: false })
  return result.filter((candidate) => candidate.needle.length >= 2)
}

function verticalPosition(items: PdfTextItem[], spans: PdfTextSpan[]) {
  return spans.reduce((sum, span) => sum + (items[span.itemIndex]?.transform[5] ?? 0), 0) / Math.max(spans.length, 1)
}

// Finds every snippet on the page. The longest snippet is anchored first; a
// snippet that occurs several times (an amount such as "650,00 lei") takes the
// occurrence closest vertically to the anchor, i.e. the same table row.
export function findSnippets(items: PdfTextItem[], snippets: string[]): PdfSnippetMatch[] {
  const indexed = indexItems(items)
  const ordered = [...new Set(snippets.map((snippet) => snippet.trim()).filter(Boolean))].sort((left, right) => normalizeForSearch(right).length - normalizeForSearch(left).length)
  const matches: PdfSnippetMatch[] = []
  let anchor: number | undefined
  for (const snippet of ordered) {
    for (const { needle, exact } of needles(snippet)) {
      const found = occurrences(indexed.text, needle).map((start) => spansFor(indexed, start, needle.length))
      if (!found.length) continue
      const best = anchor === undefined ? found[0] : found.reduce((closest, candidate) => Math.abs(verticalPosition(items, candidate) - anchor!) < Math.abs(verticalPosition(items, closest) - anchor!) ? candidate : closest)
      anchor ??= verticalPosition(items, best)
      matches.push({ snippet, spans: best, exact })
      break
    }
  }
  return matches
}

export interface HighlightRect { left: number; top: number; width: number; height: number }

// Converts matched spans to rectangles in viewport (CSS pixel) space.
// `transform` is the viewport transform; partial items are cut proportionally
// to the matched characters.
export function spanRects(items: PdfTextItem[], spans: PdfTextSpan[], transform: number[], scale: number): HighlightRect[] {
  return spans.flatMap((span) => {
    const item = items[span.itemIndex]
    if (!item || !item.str.length) return []
    const [a, b, c, d, e, f] = transform
    const [ia, ib, ic, id, ie, iF] = item.transform
    const tx = [a * ia + c * ib, b * ia + d * ib, a * ic + c * id, b * ic + d * id, a * ie + c * iF + e, b * ie + d * iF + f]
    const fontHeight = Math.hypot(tx[2], tx[3]) || item.height * scale
    const width = item.width * scale
    const length = Array.from(item.str).length
    const left = tx[4] + width * (span.start / length)
    return [{ left, top: tx[5] - fontHeight, width: Math.max(width * ((span.end - span.start) / length), 4), height: fontHeight * 1.2 }]
  })
}
