import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider, createBrowserRouter } from 'react-router'
import './theme/tokens.css'
import './theme/base.css'
import './theme/theme.ts'
import App from './App.tsx'

// A data router, which a page needs to hold a navigation back; the app's own routes are below it.
const router = createBrowserRouter([{ path: '*', element: <App /> }])

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
