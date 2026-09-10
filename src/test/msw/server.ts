import { setupServer } from 'msw/node'

// Module 1 uses the repository adapter directly. The MSW server is ready for
// future Axios-backed mock repositories without introducing a fake REST API now.
export const server = setupServer()

