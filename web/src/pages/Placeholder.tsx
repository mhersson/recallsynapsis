import PageHead from '../components/PageHead.tsx'

// Stands in for a page that is not built yet.
export default function Placeholder({ title }: { title: string }) {
  return (
    <div className="page narrow">
      <PageHead title={title} />
    </div>
  )
}
