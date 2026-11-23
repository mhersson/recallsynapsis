import { useId } from 'react'
import type { Area } from '../../api/types.ts'
import Seg from '../../components/Seg.tsx'
import type { Now, Task } from './api.ts'
import { period, shortcuts } from './dates.ts'

// What a task's form holds of its plan, its deadline and its list.
export type Plan = Pick<Task, 'planned_from' | 'planned_to' | 'planned_time' | 'deadline' | 'list'>

type ShortcutsProps = { now: Now; from: string; to: string; onPick: (from: string, to: string) => void }

// The shortcuts for when, each naming the dates it plans; the one whose dates are given is current.
export function Shortcuts({ now, from, to, onPick }: ShortcutsProps) {
  return (
    <div className="scs">
      {shortcuts(now).map(([label, a, b]) => {
        const on = from === a && to === b
        return (
          <button key={label} type="button" className={on ? 'sc on' : 'sc'} aria-current={on || undefined} onClick={() => onPick(a, b)}>
            <b>{label}</b>
            <span>{period(a, b)}</span>
          </button>
        )
      })}
    </div>
  )
}

// Planned, with its shortcuts, Deadline and List, as a task's forms show them; a change of one changes the form alone.
export function PlanFields({ value, now, onChange }: { value: Plan; now: Now; onChange: (p: Partial<Plan>) => void }) {
  const ids = { planned: useId(), deadline: useId() }
  return (
    <>
      <div className="tf">
        <span className="lb" id={ids.planned}>
          Planned
        </span>
        <div role="group" aria-labelledby={ids.planned}>
          <Shortcuts now={now} from={value.planned_from} to={value.planned_to} onPick={(a, b) => onChange({ planned_from: a, planned_to: b })} />
          <div className="dts">
            <label>
              From
              {/* A one-day plan stays one day, and a span keeps its to: a date typed a part at a time passes through others. */}
              <input
                type="date"
                value={value.planned_from}
                onChange={(e) => onChange({ planned_from: e.target.value, planned_to: value.planned_to === value.planned_from ? e.target.value : value.planned_to })}
              />
            </label>
            <label>
              To
              <input type="date" value={value.planned_to} min={value.planned_from} disabled={!value.planned_from} onChange={(e) => onChange({ planned_to: e.target.value })} />
            </label>
            <label>
              Starts at
              <input type="time" value={value.planned_time} disabled={!value.planned_from} onChange={(e) => onChange({ planned_time: e.target.value })} />
            </label>
            {value.planned_from && (
              <button type="button" className="btn link" onClick={() => onChange({ planned_from: '', planned_to: '', planned_time: '' })}>
                Clear the plan
              </button>
            )}
          </div>
        </div>
      </div>
      <div className="tf">
        <label className="lb" htmlFor={ids.deadline}>
          Deadline
        </label>
        <div className="acts">
          <input id={ids.deadline} type="date" value={value.deadline} onChange={(e) => onChange({ deadline: e.target.value })} />
          {value.deadline && (
            <button type="button" className="btn link" aria-label="Clear the deadline" onClick={() => onChange({ deadline: '' })}>
              Clear
            </button>
          )}
        </div>
      </div>
      <div className="tf">
        <span className="lb">List</span>
        <Seg
          label="List"
          options={[
            ['', 'None'],
            ['soon', 'Soon'],
            ['someday', 'Someday'],
          ]}
          value={value.list}
          onChange={(list) => onChange({ list })}
        />
      </div>
    </>
  )
}

// The area of a task, chosen among those given.
export function AreaField({ value, areas, onChange }: { value: string; areas: Area[]; onChange: (area: string) => void }) {
  const id = useId()
  return (
    <div className="tf">
      <label className="lb" htmlFor={id}>
        Area
      </label>
      <div>
        <select id={id} value={value} onChange={(e) => onChange(e.target.value)}>
          {areas.map((a) => (
            <option key={a.slug} value={a.slug}>
              {a.name}
            </option>
          ))}
        </select>
      </div>
    </div>
  )
}
