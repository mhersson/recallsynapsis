import type { ReactNode } from 'react'

// A line icon on a 16 unit grid, drawn in the colour of its text.
export function Icon({ children }: { children: ReactNode }) {
  return (
    <svg className="ic" viewBox="0 0 16 16" aria-hidden="true">
      {children}
    </svg>
  )
}

export const ProjectsIcon = () => (
  <Icon>
    <path d="M2 4h4l1.3 1.6H14V12.5H2z" />
  </Icon>
)

export const SearchIcon = () => (
  <Icon>
    <circle cx="7" cy="7" r="4" />
    <path d="m10.2 10.2 3.3 3.3" />
  </Icon>
)

export const MoreIcon = () => (
  <Icon>
    <circle cx="3.5" cy="8" r=".6" />
    <circle cx="8" cy="8" r=".6" />
    <circle cx="12.5" cy="8" r=".6" />
  </Icon>
)

export const SettingsIcon = () => (
  <Icon>
    <path d="M2.5 5h6M12.5 5h1M2.5 11h1M7.5 11h6" />
    <circle cx="10.5" cy="5" r="1.6" />
    <circle cx="5.5" cy="11" r="1.6" />
  </Icon>
)

export const AddIcon = () => (
  <Icon>
    <path d="M8 3v10M3 8h10" />
  </Icon>
)
