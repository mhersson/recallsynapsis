import './AccessEnded.css'

// No scrim: the page stays readable, so a person can copy what they typed before they reload.
export default function AccessEnded() {
  return (
    <div className="access-ended" role="alert">
      <p>Access to this page has ended. Reload to sign in again.</p>
      <button className="btn" onClick={() => window.location.reload()}>
        Reload
      </button>
    </div>
  )
}
