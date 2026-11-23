import { api, ApiError } from '../../api/client.ts'
import { store, stored } from '../../components/stored.ts'

// A browser of the caller as GET /api/push?browser=ID answers it: failed_at is null when the last message went through.
export type Browser = { id: number; failed_at: string | null; failure: string }

// GET /api/push: the key a browser subscribes with, in how many browsers notifications are on, and this one when it is on.
type Push = { key: string; on: number; browser: Browser | null }

// This browser: with no Web Push, an iPhone's or iPad's Safari tab, refusing notifications, or off or on.
export type State = 'none' | 'home' | 'refused' | 'off' | 'on'

export type Seen = Push & { state: State }

const ID = 'push_browser'

const able = () => isSecureContext && 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window

// ready never settles before a worker is registered, and the page registers one only as notifications are turned on.
const subscription = async () => (able() ? ((await navigator.serviceWorker.getRegistration())?.pushManager.getSubscription() ?? null) : null)

// Every step runs after the one before, so a look never sees Turn on half done and unsubscribes what it subscribed.
let chain: Promise<unknown> = Promise.resolve()
function queue<T>(step: () => Promise<T>): Promise<T> {
  const p = chain.then(step)
  chain = p.catch(() => {})
  return p
}

// Removes a browser at the server, where a 404 means it is gone already, and forgets its id.
async function remove(id: string) {
  try {
    await api('DELETE', `/push/${id}`)
  } catch (e) {
    if (!(e instanceof ApiError && e.status === 404)) throw e
  }
  store(ID, null)
}

// The page never registers by itself, which would undo Turn off everywhere: a subscription the server does not know is
// unsubscribed, and a kept id with no subscription is removed at the server.
export const look = () =>
  queue(async (): Promise<Seen> => {
    if (!able()) {
      // On iOS, navigator.standalone tells a Safari tab from a Home Screen app, and push is the Home Screen app's.
      const home = isSecureContext && (navigator as { standalone?: boolean }).standalone === false
      return { ...(await api<Push>('GET', '/push')), state: home ? 'home' : 'none' }
    }
    if (Notification.permission === 'denied') return { ...(await api<Push>('GET', '/push')), state: 'refused' }
    const s = await subscription()
    let id = stored(ID)
    if (id && !s) {
      await remove(id)
      id = null
    }
    const push = await api<Push>('GET', id ? `/push?browser=${id}` : '/push')
    if (s && !push.browser) {
      await s.unsubscribe()
      store(ID, null)
    }
    return { ...push, state: push.browser ? 'on' : 'off' }
  })

// The key arrives as unpadded base64url.
const bytes = (key: string) => Uint8Array.from(atob(key.replace(/-/g, '+').replace(/_/g, '/')), (c) => c.charCodeAt(0))

// Called inside the click: the permission is asked before anything is awaited, as Safari demands. Its answer is awaited
// outside the queue, since a prompt left open would otherwise hold up every later step, signing out among them.
export async function turnOn(key: string): Promise<void> {
  if ((await Notification.requestPermission()) !== 'granted') return
  return queue(async () => {
    await navigator.serviceWorker.register('/sw.js')
    const s = await (await navigator.serviceWorker.ready).pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: bytes(key) })
    // The server refuses unknown fields, so only these are sent.
    const { endpoint, keys } = s.toJSON()
    try {
      const { id } = await api<{ id: number }>('PUT', '/push', { endpoint, keys: { p256dh: keys?.p256dh, auth: keys?.auth } })
      store(ID, String(id))
    } catch (e) {
      await s.unsubscribe()
      throw e
    }
  })
}

// The server forgets the browser even when the browser fails to unsubscribe, so a sign-out never leaves it sending here.
export const turnOff = () =>
  queue(async () => {
    try {
      await (await subscription())?.unsubscribe()
    } finally {
      const id = stored(ID)
      if (id) await remove(id)
    }
  })

export const offEverywhere = () =>
  queue(async () => {
    await api('DELETE', '/push')
    await (await subscription())?.unsubscribe()
    store(ID, null)
  })
