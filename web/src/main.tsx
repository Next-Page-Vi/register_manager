import { QueryClientProvider } from '@tanstack/react-query'
import { queryClient } from '@/api/queries'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@/i18n'
import './index.css'
import { App } from './App'
import { ErrorBoundary } from '@/components/ErrorBoundary'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </ErrorBoundary>
  </StrictMode>,
)
