import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { ResourceProvider } from './currency/context/ResourceContext.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ResourceProvider>
      <App />
    </ResourceProvider>
  </StrictMode>,
)
