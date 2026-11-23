// The ring before a task, which marks it done and, filled once it is, opens it again; it has the yellow of what waits
// while the task is to sort.
export default function Check({ done, sort, big, onClick }: { done: boolean; sort: boolean; big?: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      className={'chk' + (big ? ' big' : '') + (done ? ' done' : sort ? ' sort' : '')}
      aria-pressed={done}
      aria-label={done ? 'Open it again' : 'Mark it done'}
      title={done ? 'Open it again' : 'Mark it done'}
      onClick={onClick}
    >
      {done && (
        <svg viewBox="0 0 12 12" aria-hidden="true">
          <path d="m2.6 6.3 2.3 2.3 4.6-4.9" />
        </svg>
      )}
    </button>
  )
}
