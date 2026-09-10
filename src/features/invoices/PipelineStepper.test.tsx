import { render, screen } from '@testing-library/react'
import { PipelineStepper } from './PipelineStepper'

describe('PipelineStepper', () => {
  it('renders the active blocking state and completed stages', () => {
    const { container } = render(<PipelineStepper invoice={{
      pipelineStatus: 'AWAITING_MATCH_CONFIRM',
      pipelinePath: ['DOWNLOADED', 'ARCHIVED', 'MATCHING', 'AWAITING_MATCH_CONFIRM', 'DEDUPE_CHECKED', 'EXPORTED'],
      sagaStatus: 'NOT_READY',
    }} />)

    expect(screen.getByText('Procesare oprită.')).toBeInTheDocument()
    expect(screen.getByText('Contabilul trebuie să confirme contractul.')).toBeInTheDocument()
    expect(container.querySelectorAll('[data-state="completed"]')).toHaveLength(3)
    expect(container.querySelectorAll('[data-state="blocked"]')).toHaveLength(1)
  })

  it('represents duplicate as a terminal state', () => {
    render(<PipelineStepper invoice={{
      pipelineStatus: 'DUPLICATE',
      pipelinePath: ['DOWNLOADED', 'ARCHIVED', 'MATCHING', 'DEDUPE_CHECKED', 'DUPLICATE'],
      sagaStatus: 'NOT_READY',
    }} />)

    expect(screen.getByRole('heading', { name: 'Duplicat' })).toBeInTheDocument()
    expect(screen.getByText('Stare terminală')).toBeInTheDocument()
  })
})

