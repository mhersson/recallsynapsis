import { useLocation, useNavigate } from 'react-router'

// The state a link of the app gives the page it opens, so the page knows there is a page of the app to go back to.
export const back = { back: true }

// Closes a page by going back to where it was opened from, so a step back never opens it again. Opened by its address,
// or again after a sign-in, there is no page of the app to go back to, so it closes to the page given, in its place.
export function useClose(to: string): () => void {
  const navigate = useNavigate()
  const { state } = useLocation()
  return () => ((state as typeof back | null)?.back ? navigate(-1) : navigate(to, { replace: true }))
}
