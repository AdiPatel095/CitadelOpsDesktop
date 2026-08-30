import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './MaterialExpressive.css'
import App from './App.tsx'

const preloadRecoveryKey = 'citadelops:preload-recovery-bundle'

window.addEventListener('vite:preloadError', (event) => {
  event.preventDefault()
  const currentBundle = import.meta.url
  try {
    if (window.sessionStorage.getItem(preloadRecoveryKey) === currentBundle) return
    window.sessionStorage.setItem(preloadRecoveryKey, currentBundle)
  } catch {
    // Storage can be unavailable in privacy-restricted browser sessions. A
    // reload is still the safest recovery from a replaced hashed asset.
  }
  window.location.reload()
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
