import { useLayoutEffect } from 'react'
import { act, render } from '@testing-library/react'
import { MemoryRouter, useLocation, useNavigate } from 'react-router'
import App from './App.tsx'

// Opens the whole app at path, as a person who follows a link there; where() tells the path it shows now, and back()
// steps back, as the browser's Back does. A navigation after the server answers an event the test fired renders outside
// act and may end after the event, so a test waits for it.
export async function open(path: string) {
  let here = ''
  let goBack = () => {}
  function Here() {
    const { pathname, search } = useLocation()
    const navigate = useNavigate()
    useLayoutEffect(() => {
      here = pathname + search
      goBack = () => void navigate(-1)
    })
    return null
  }
  const view = render(
    <MemoryRouter initialEntries={[path]}>
      <App />
      <Here />
    </MemoryRouter>,
  )
  await act(async () => {})
  return { ...view, where: () => here, back: () => act(async () => goBack()) }
}
