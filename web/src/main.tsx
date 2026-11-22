import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './theme/tokens.css'
import './theme/base.css'
import './theme/theme.ts'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
