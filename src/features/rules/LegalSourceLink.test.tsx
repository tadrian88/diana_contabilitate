import { render, screen, cleanup } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { LegalSourceLink } from './LegalSourceLink'

afterEach(cleanup)
describe('Legal source provenance links', () => {
  it('opens official sources with an isolated external browsing context', () => {
    render(<LegalSourceLink url="https://static.anaf.ro/static/law.pdf" />)
    const link = screen.getByRole('link', { name: 'Consultă sursa oficială externă' })
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })
  it.each(['javascript:alert(1)', 'https://static.anaf.ro.example.com/law', 'https://user@static.anaf.ro/law'])('rejects unsafe provenance URL %s', (url) => {
    render(<LegalSourceLink url={url} />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
