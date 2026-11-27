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

export const XIcon = () => (
  <Icon>
    <path d="m4.5 4.5 7 7M11.5 4.5l-7 7" />
  </Icon>
)

export const LinkIcon = () => (
  <Icon>
    <path d="M6.8 9.2a2.6 2.6 0 0 0 3.7 0l2.2-2.2a2.6 2.6 0 0 0-3.7-3.7l-.9.9" />
    <path d="M9.2 6.8a2.6 2.6 0 0 0-3.7 0L3.3 9a2.6 2.6 0 0 0 3.7 3.7l.9-.9" />
  </Icon>
)

export const TrashIcon = () => (
  <Icon>
    <path d="M2.5 4.5h11M6.5 4.5V2.8h3v1.7M4 4.5l.7 9h6.6l.7-9" />
  </Icon>
)

export const RestoreIcon = () => (
  <Icon>
    <path d="M3.2 8a4.8 4.8 0 1 0 1.4-3.4" />
    <path d="M3.5 2.5v2.8h2.8" />
  </Icon>
)

export const HistoryIcon = () => (
  <Icon>
    <circle cx="8" cy="8" r="5.5" />
    <path d="M8 5v3.2l2 1.3" />
  </Icon>
)
