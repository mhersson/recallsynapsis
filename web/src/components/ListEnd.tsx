type Props = { shown: number; total: number; what: string; more?: () => void }

// The end of a list that holds more than it shows, the most recently what first, with a way to show more while the list
// may give more.
export default function ListEnd({ shown, total, what, more }: Props) {
  if (total <= shown) return null
  return (
    <div className="list-end">
      <span>
        These are the {shown} most recently {what}. There are more.
      </span>
      {more && (
        <button className="btn" onClick={more}>
          Show 50 more
        </button>
      )}
    </div>
  )
}
