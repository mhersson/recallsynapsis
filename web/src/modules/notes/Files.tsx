import { useRef, useState, type ChangeEvent } from 'react'
import { api, message } from '../../api/client.ts'
import Dialog from '../../components/Dialog.tsx'
import { useNarrow } from '../../components/useNarrow.ts'
import type { Attached, Note } from './api.ts'
import { AttachIcon, CameraIcon, DownloadIcon, XIcon } from './icons.tsx'
import Section from './Section.tsx'
import { size, type Uploads } from './uploads.ts'

// The types the server answers as themselves, inline; it answers any other as a download.
const images = ['image/png', 'image/jpeg', 'image/gif', 'image/webp']

// What a file is, by the end of its name.
const ext = (name: string) => /\.(\w{1,8})$/.exec(name)?.[1].toLowerCase() ?? 'file'

type Props = { note: Note; trashed: boolean; up: Uploads; onRemoved: () => void }

// The note's files: the server's inline images as images that open to fill the view, any other as a download, and a
// removal asked first, every time. A note in the trash changes none, and shows the section only when it has one.
export default function Files({ note, trashed, up, onRemoved }: Props) {
  const narrow = useNarrow()
  const [opened, setOpened] = useState<Attached>()
  const [removing, setRemoving] = useState<Attached>()
  const picker = useRef<HTMLInputElement>(null)
  const camera = useRef<HTMLInputElement>(null)
  const at = (f: Attached) => `/notes/${note.id}/files/${f.id}`
  // Emptied, so the same file can be chosen again.
  const chosen = (e: ChangeEvent<HTMLInputElement>) => {
    up.send([...(e.target.files ?? [])])
    e.target.value = ''
  }
  const remove = (f: Attached) =>
    api('DELETE', at(f)).then(
      () => {
        up.refuse('')
        onRemoved()
      },
      (e: unknown) => up.refuse(message(e)),
    )

  if (trashed && note.files.length === 0) return null
  const add = !trashed && !narrow && (
    <button className="btn link" onClick={() => picker.current!.click()}>
      <AttachIcon />
      Attach a file
    </button>
  )
  const download = (f: Attached) => (
    <a className="btn" href={'/api' + at(f)} download>
      <DownloadIcon />
      Download
    </a>
  )
  const n = up.going.length
  return (
    <Section title="Attachments" count={note.files.length + n} add={add} open>
      <div className="tiles">
        {note.files.map((f) => (
          <div key={f.id} className="tile">
            {images.includes(f.type) ? (
              <button className="tile-open" onClick={() => setOpened(f)}>
                <img src={'/api' + at(f)} alt="" loading="lazy" />
                <span className="nm">{f.name}</span>
              </button>
            ) : (
              <a className="tile-open" href={'/api' + at(f)} download>
                <span className="doc">
                  {ext(f.name)} · {size(f.size)}
                </span>
                <span className="nm">{f.name}</span>
              </a>
            )}
            {!trashed && (
              <button className="x" aria-label={`Remove ${f.name}`} onClick={() => setRemoving(f)}>
                <XIcon />
              </button>
            )}
          </div>
        ))}
        {up.going.map((u) => (
          <div key={u.key} className="tile up" aria-busy="true">
            <span className="doc">
              {ext(u.name)} · {size(u.size)}
            </span>
            <span className="bar" />
            <span className="nm">{u.name}</span>
          </div>
        ))}
      </div>
      {n > 0 && (
        <p className="fline">
          <AttachIcon />
          {n === 1 ? 'Uploading 1 file. Leaving the note stops it.' : `Uploading ${n} files. Leaving the note stops them.`}
        </p>
      )}
      {up.refused && (
        <p role="alert" className="err">
          {up.refused}
        </p>
      )}
      {!trashed &&
        (narrow ? (
          <div className="acts two">
            <button className="btn" onClick={() => camera.current!.click()}>
              <CameraIcon />
              Take a photo
            </button>
            <button className="btn" onClick={() => picker.current!.click()}>
              <AttachIcon />
              Choose a file
            </button>
          </div>
        ) : (
          <p className="note">Drop a file on the note, or paste an image, to attach it.{up.cap > 0 && ` At most ${size(up.cap)} each.`}</p>
        ))}
      {!trashed && (
        <>
          <input ref={picker} type="file" multiple hidden onChange={chosen} />
          {narrow && <input ref={camera} type="file" accept="image/*" capture="environment" hidden onChange={chosen} />}
        </>
      )}
      {opened && (
        <Dialog title={opened.name} className="viewer" onClose={() => setOpened(undefined)}>
          <div className="vbar">
            <span className="t2">
              <span>{ext(opened.name)}</span>
              <span>{size(opened.size)}</span>
            </span>
            <span className="spacer" />
            {!narrow && download(opened)}
            <button className="btn" onClick={() => setOpened(undefined)}>
              {!narrow && <XIcon />}
              Close
            </button>
          </div>
          <img src={'/api' + at(opened)} alt="" />
          {narrow && <div className="vfoot">{download(opened)}</div>}
        </Dialog>
      )}
      {removing && (
        <Dialog title={`Remove ${removing.name}?`} onClose={() => setRemoving(undefined)}>
          <p className="note">The file is removed from the note for good. This cannot be undone.</p>
          <div className="acts">
            <button className="btn" onClick={() => setRemoving(undefined)}>
              Keep it
            </button>
            <button
              className="btn danger"
              onClick={() => {
                setRemoving(undefined)
                void remove(removing)
              }}
            >
              Remove
            </button>
          </div>
        </Dialog>
      )}
    </Section>
  )
}
