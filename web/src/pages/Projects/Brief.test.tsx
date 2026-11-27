import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Area, Session } from '../../api/types.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow, type Call } from '../../test-server.ts'
import type { Wording } from './Brief.tsx'

afterEach(() => vi.unstubAllGlobals())

const session: Session = { user: { name: 'ada' }, modules: [], version: '1.2.3', timezone: 'UTC', last_backup: null }
const area = (id: number, kind: Area['kind'], brief: string, revision = brief ? 1 : 0): Area => ({
  id,
  kind,
  name: kind,
  slug: kind,
  brief,
  brief_revision: revision,
  brief_changed_at: revision ? '2026-11-20T10:00:00Z' : null,
  brief_changed_by: revision ? 'web' : '',
  archived: false,
})
const wording = (revision: number, brief: string, w: Partial<Wording> = {}): Wording => ({
  revision,
  at: '2026-11-20T10:00:00Z',
  by: 'web',
  said: '',
  restored_from: null,
  brief,
  ...w,
})

// A project at revision 3, whose second wording a client drafted with what the owner said.
const project = () => area(2, 'project', 'the brief **now**', 3)
const drafts = () => [wording(1, 'one line'), wording(2, 'drafted\n\nby a client', { by: 'aim', said: 'draft it' }), wording(3, 'the brief **now**')]

// A server that keeps each area's wordings, the oldest first, and restores one as a new revision that names it.
function server(areas: Area[], kept: Record<number, Wording[]> = {}) {
  return fakeServer((c) => {
    if (c.path === '/api/session') return json(session)
    if (c.path.startsWith('/api/areas?') || c.path === '/api/areas') return json({ areas })
    const [, id, n, restore] = /^\/api\/areas\/(\d+)\/revisions(?:\/(\d+)(\/restore)?)?$/.exec(c.path) ?? []
    const a = areas.find((x) => x.id === Number(id))
    const list = kept[Number(id)] ?? []
    if (!a) return
    if (!n) return json({ revisions: list.map(({ brief: _, ...e }) => e).reverse() })
    const w = list.find((x) => x.revision === Number(n))
    if (!w) return
    if (!restore) return json(w)
    Object.assign(a, { brief: w.brief, brief_revision: a.brief_revision + 1 })
    list.push(wording(a.brief_revision, w.brief, { restored_from: w.revision }))
    return json(a)
  })
}

const main = () => screen.getByRole('main')
const brief = () => main().querySelector<HTMLElement>('.brief')!
const shown = () => [...brief().querySelectorAll(':scope > .md > *')]
const history = () => within(brief()).getByRole('button', { expanded: false })
const rows = () => [...brief().querySelectorAll<HTMLElement>('.hrow')]
const frame = () => main().querySelector<HTMLElement>('.revision')
const changes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path])

test('a brief of more than one paragraph shows the first as Markdown, unfolds the rest in place and folds it again', async () => {
  narrow(false)
  server([area(1, 'personal', 'the **first**\n\n- a\n- b\n\nthe last')])
  await open('/areas/1')
  expect(shown()).toHaveLength(1)
  expect(brief().querySelector('.md strong')).toHaveTextContent('first')
  await userEvent.click(within(brief().querySelector<HTMLElement>('.md')!).getByRole('button'))
  expect(shown().map((b) => b.tagName)).toEqual(['P', 'UL', 'P'])
  await userEvent.click(within(brief().querySelector<HTMLElement>('.md')!).getByRole('button'))
  expect(shown()).toHaveLength(1)
})

test('a brief of one paragraph shows whole, with no way to more', async () => {
  narrow(false)
  server([area(1, 'personal', ''), area(2, 'project', 'all of it, *here*\non two lines')])
  await open('/areas/2')
  expect(shown()).toHaveLength(1)
  expect(brief().querySelector('.md em')).toHaveTextContent('here')
  expect(brief().querySelector('.md button')).toBeNull()
})

test('an area with no brief says so, and has History only once a brief was written', async () => {
  narrow(true)
  server([area(1, 'personal', ''), area(2, 'project', '', 2)], { 2: [wording(1, 'one line'), wording(2, '')] })
  const never = await open('/areas/1')
  expect(brief().querySelector('.md')).toBeNull()
  expect(brief().querySelector('.sub')).not.toBeNull()
  expect(within(brief()).queryByRole('button')).toBeNull()
  never.unmount()

  await open('/areas/2')
  expect(brief().querySelector('.sub')).not.toBeNull()
  await userEvent.click(history())
  await userEvent.click(within(rows()[1]).getByRole('button'))
  expect(await within(main()).findByText('one line')).toBeInTheDocument()
  expect(within(frame()!).getAllByRole('button')).toHaveLength(2)
})

test('an archived project\'s earlier wording is read, and not restored', async () => {
  narrow(false)
  server([area(1, 'personal', ''), { ...project(), archived: true }], { 2: drafts() })
  await open('/areas/2')
  await userEvent.click(history())
  await userEvent.click(within(rows()[2]).getByRole('button'))
  await within(main()).findByText('one line')
  expect(within(frame()!).getAllByRole('button')).toHaveLength(1)
})

test('History lists every revision the newest first, with what the owner said, and Read shows that wording and no other', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', ''), project()], { 2: drafts() })
  await open('/areas/2')
  await userEvent.click(history())
  expect(rows().map((r) => within(r).queryAllByRole('button').length)).toEqual([0, 1, 1])
  expect(rows().map((r) => !!r.querySelector('.said'))).toEqual([false, true, false])

  await userEvent.click(within(rows()[2]).getByRole('button'))
  expect(calls.at(-1)!.path).toBe('/api/areas/2/revisions/1')
  expect(await within(main()).findByText('one line')).toBeInTheDocument()
  expect(frame()!.querySelector('.rev .md')!.textContent).toBe('one line')
  expect(main().querySelector('.brief > .md')).toBeNull()

  await userEvent.click(within(frame()!).getAllByRole('button').at(-1)!)
  expect(frame()).toBeNull()
  await userEvent.click(within(rows()[1]).getByRole('button'))
  expect(calls.at(-1)!.path).toBe('/api/areas/2/revisions/2')
  expect(await within(main()).findByText('by a client')).toBeInTheDocument()
  expect(frame()!.querySelector('.rev .md')!.textContent).toBe('draftedby a client')
  expect(changes(calls)).toEqual([])
})

test('an earlier wording the brief has now is read, and not restored, since restoring it would write nothing', async () => {
  narrow(false)
  server([area(1, 'personal', ''), project()], { 2: [wording(1, 'the brief **now**'), wording(2, 'drafted'), wording(3, 'the brief **now**')] })
  await open('/areas/2')
  await userEvent.click(history())
  await userEvent.click(within(rows()[2]).getByRole('button'))
  await waitFor(() => expect(frame()!.querySelector('.rev')).not.toBeNull())
  expect(within(frame()!).getAllByRole('button')).toHaveLength(1)
})

test('an earlier wording is read where the brief stands, so the page stays where it is', async () => {
  narrow(false)
  const scrolled = vi.fn()
  Element.prototype.scrollIntoView = scrolled
  try {
    server([area(1, 'personal', ''), project()], { 2: drafts() })
    await open('/areas/2')
    await userEvent.click(history())
    await userEvent.click(within(rows()[2]).getByRole('button'))
    await within(main()).findByText('one line')
    expect(scrolled).not.toHaveBeenCalled()
  } finally {
    Reflect.deleteProperty(Element.prototype, 'scrollIntoView')
  }
})

test('Restore sends the restore, then shows the brief as restored, one revision on', async () => {
  narrow(true)
  const calls = server([area(1, 'personal', ''), project()], { 2: drafts() })
  await open('/areas/2')
  await userEvent.click(history())
  await userEvent.click(within(rows()[2]).getByRole('button'))
  await within(main()).findByText('one line')
  await userEvent.click(within(frame()!).getAllByRole('button')[0])
  expect(changes(calls)).toEqual([['POST', '/api/areas/2/revisions/1/restore']])
  expect(frame()).toBeNull()
  expect(brief().querySelector('.md')!.textContent).toBe('one line')
  await userEvent.click(history())
  await waitFor(() => expect(rows()).toHaveLength(4))
  expect(rows().map((r) => within(r).queryAllByRole('button').length)).toEqual([0, 1, 1, 1])
})

test('a brief changed elsewhere shows without a reload, and its History is read again', async () => {
  vi.useFakeTimers()
  try {
    narrow(false)
    const areas = [area(1, 'personal', ''), project()]
    const kept = { 2: drafts() }
    server(areas, kept)
    await open('/areas/2')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    await act(async () => fireEvent.click(history()))
    expect(rows()).toHaveLength(3)

    Object.assign(areas[1], { brief: 'mended **elsewhere**', brief_revision: 4 })
    kept[2].push(wording(4, 'mended **elsewhere**', { by: 'aim' }))
    act(() => es.send({ module: 'core', kind: 'area', id: 2, area: 2 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(brief().querySelector('.md strong')).toHaveTextContent('elsewhere')
    expect(rows()).toHaveLength(4)
  } finally {
    vi.useRealTimers()
  }
})
