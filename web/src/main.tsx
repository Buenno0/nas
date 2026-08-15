import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ApiError } from './lib/api'
import { ThemeProvider } from './lib/theme'
import { PlayerProvider } from './lib/player'
import App from './App'
import './index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      // Repetir um 401 é inútil: a sessão não vai voltar sozinha.
      retry: (count, error) => !(error instanceof ApiError && error.status === 401) && count < 2,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <PlayerProvider>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </PlayerProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
)
