import { findSnippets, normalizeForSearch, spanRects, type PdfTextItem } from './pdfTextSearch'

const item = (str: string, y: number, x = 50): PdfTextItem => ({ str, transform: [10, 0, 0, 10, x, y], width: str.length * 5, height: 10 })

// Page 1 of the BG-2025-117 tariff table as pdf.js streams it: a wrapped cell
// ("… Business 2" / "VM") is split by the other cells of its row.
const tariffTable = [
  item('1', 600), item('Mentenanță IT — abonament lunar', 600), item('lună', 600), item('1.800,00 lei', 600),
  item('2', 560), item('Hosting cloud — pachet Business 2', 560), item('lună', 560), item('650,00 lei', 560), item('1 lună', 560),
  item('VM', 548),
  item('3', 520), item('Intervenții suplimentare peste', 520), item('oră', 520), item('150,00 lei', 520),
  item('4.2. Valoarea contractului: 650,00 lei pentru găzduire.', 300),
]

it('matches regardless of case, whitespace, dashes and cedilla or comma diacritics', () => {
  expect(normalizeForSearch('Mentenanţă  IT – abonament')).toBe(normalizeForSearch('mentenanță it — ABONAMENT'))
  const [match] = findSnippets([item('Prețurile nu includ', 200), item('TVA.', 200)], ['Preţurile nu includ TVA.'])
  expect(match.exact).toBe(true)
  expect(match.spans.map((span) => span.itemIndex)).toEqual([0, 1])
})

it('highlights the price on the same row as the service when the amount occurs twice', () => {
  const matches = findSnippets(tariffTable, ['650,00 lei', 'Hosting cloud — pachet Business 2 VM'])
  const price = matches.find((match) => match.snippet === '650,00 lei')!
  const service = matches.find((match) => match.snippet.startsWith('Hosting'))!
  expect(service.exact).toBe(false)
  expect(service.spans[0].itemIndex).toBe(5)
  expect(price.spans).toEqual([{ itemIndex: 7, start: 0, end: 10 }])
})

it('returns nothing for text that is not on the page', () => {
  expect(findSnippets(tariffTable, ['Servicii de contabilitate'])).toEqual([])
})

it('cuts a partially matched item proportionally', () => {
  const items = [item('ABCDEFGHIJ', 100, 0)]
  const [match] = findSnippets(items, ['DEF'])
  const [rect] = spanRects(items, match.spans, [1, 0, 0, -1, 0, 800], 1)
  expect(rect.left).toBeCloseTo(15)
  expect(rect.width).toBeCloseTo(15)
  expect(rect.top).toBeCloseTo(690)
})
