// A count that only informs, quiet after its word, and nothing at zero, so a bare word means empty. A number the owner
// must act on has the filled badge, .cnt, instead.
export default function Count({ children }: { children: number | string }) {
  return children ? <span className="count">{children}</span> : null
}
