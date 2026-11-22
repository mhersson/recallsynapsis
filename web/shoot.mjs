// Shoots a page with headless Firefox: node shoot.mjs WIDTH HEIGHT URL FILE.png
// Firefox's own --screenshot fires at the load event, before the app has read its data; this waits a second more.
import { spawn } from 'node:child_process'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const [width, height, url, file] = process.argv.slice(2)
const profile = mkdtempSync(join(tmpdir(), 'shoot-'))
const firefox = spawn('firefox', ['--headless', '--no-remote', '--profile', profile, '--remote-debugging-port', '0'])

try {
  const address = await new Promise((resolve, reject) => {
    firefox.on('exit', () => reject(new Error('firefox exited')))
    firefox.stderr.on('data', (d) => {
      const m = /WebDriver BiDi listening on (ws:\/\/\S+)/.exec(String(d))
      if (m) resolve(m[1])
    })
  })
  const ws = new WebSocket(address + '/session')
  await new Promise((resolve) => ws.addEventListener('open', resolve))
  let id = 0
  const call = (method, params) =>
    new Promise((resolve, reject) => {
      const n = ++id
      ws.addEventListener('message', function on(m) {
        const r = JSON.parse(m.data)
        if (r.id !== n) return
        ws.removeEventListener('message', on)
        if (r.type === 'error') reject(new Error(`${method}: ${r.message}`))
        else resolve(r.result)
      })
      ws.send(JSON.stringify({ id: n, method, params }))
    })

  await call('session.new', { capabilities: {} })
  const { context } = await call('browsingContext.create', { type: 'tab' })
  await call('browsingContext.setViewport', { context, viewport: { width: Number(width), height: Number(height) }, devicePixelRatio: 1 })
  await call('browsingContext.navigate', { context, url, wait: 'complete' })
  await new Promise((resolve) => setTimeout(resolve, 1000))
  const { data } = await call('browsingContext.captureScreenshot', { context })
  writeFileSync(file, Buffer.from(data, 'base64'))
  ws.close()
} finally {
  firefox.kill()
  rmSync(profile, { recursive: true, force: true })
}
