import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PermissionProvider } from './contexts/PermissionContext'
import App from './App'
import './index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchInterval: 30000, // Refresh every 30s
      staleTime: 10000,
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <PermissionProvider>
          <App />
        </PermissionProvider>
      </BrowserRouter>
    </QueryClientProvider>
  </React.StrictMode>,
)
