import { useState } from 'react'
import { message } from '../../api/client.ts'
import { useStream } from '../../api/stream.ts'
import Dialog from '../../components/Dialog.tsx'
import { moment } from '../../components/time.ts'
import { useModules } from '../../session.ts'
import { look, offEverywhere, turnOff, turnOn, type Seen, type State } from './push.ts'
import './Notifications.css'

const lines: Record<State, string> = {
  off: 'Off in this browser.',
  on: 'On in this browser.',
  home: 'On an iPhone or iPad, add Recall Synapsis to the Home Screen first, then turn notifications on there.',
  refused: "This browser refuses notifications from this site. Allow them in the browser's settings for the site, then load the page again.",
  none: 'This browser has no Web Push, or the page is not on https.',
}

const browsers = (n: number) => (n === 1 ? 'the browser that has' : `the ${n} browsers that have`)

// The part Notifications of Settings: this browser, the browsers of the caller, and under them each module's kinds.
export default function Notifications() {
  const kinds = useModules().flatMap((m) => m.notify ?? [])
  const [seen, setSeen] = useState<Seen | null>(null)
  const [refused, setRefused] = useState('')
  const [busy, setBusy] = useState(false)
  const [asking, setAsking] = useState(false)

  const read = () => look().then(setSeen, (e: unknown) => setRefused(message(e)))
  useStream((changes) => {
    if (!changes || changes.some((c) => c.module === 'push')) void read()
  })

  // Called inside the click, so Turn on asks for the permission there.
  function act(step: () => Promise<void>) {
    setBusy(true)
    setRefused('')
    step()
      .catch((e: unknown) => setRefused(message(e)))
      .then(read)
      .finally(() => setBusy(false))
  }

  const turn = seen?.state === 'off' ? () => act(() => turnOn(seen.key)) : seen?.state === 'on' ? () => act(turnOff) : null
  const on = seen?.on ?? 0
  // The kinds hold for every browser, so they show while this one cannot be read, and stay mounted once it is.
  return (
    <>
      {seen && (
        <>
          <div className="acts stack push-line">
            <p>{lines[seen.state]}</p>
            {turn && (
              <button className="btn" disabled={busy} onClick={turn}>
                {seen.state === 'on' ? 'Turn off' : 'Turn on'}
              </button>
            )}
          </div>
          {seen.browser?.failed_at && (
            <p className="late push-failed">
              <i className="mk wait" />
              <span>
                The last notification to this browser failed, {moment(seen.browser.failed_at)}: {seen.browser.failure}.
              </span>
            </p>
          )}
        </>
      )}
      {refused && (
        <p role="alert" className="err push-refused">
          {refused}
        </p>
      )}
      {seen && (
        <div className="acts push-count">
          <p className="note">{on ? `On in ${on} browser${on === 1 ? '' : 's'} in all.` : 'On in no browser.'}</p>
          {on > 0 && (
            <button className="btn sm" onClick={() => setAsking(true)}>
              Turn off everywhere
            </button>
          )}
        </div>
      )}
      {kinds.length > 0 && (
        <div className="push-kinds">
          {kinds.map((Kinds, i) => (
            <Kinds key={i} />
          ))}
        </div>
      )}
      {asking && (
        <Dialog title="Turn notifications off everywhere?" onClose={() => setAsking(false)}>
          <p className="note">They go off in {browsers(on)} them on. Turn them on again in each browser you use.</p>
          <div className="acts">
            <button className="btn" onClick={() => setAsking(false)}>
              Keep them
            </button>
            <button
              className="btn danger"
              onClick={() => {
                setAsking(false)
                act(offEverywhere)
              }}
            >
              Turn off everywhere
            </button>
          </div>
        </Dialog>
      )}
    </>
  )
}
