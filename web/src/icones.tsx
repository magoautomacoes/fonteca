// Icones em linha, 1.5px, na cor do texto. Um conjunto so, mesmo traco.
type P = { className?: string }

const base = {
  width: 16,
  height: 16,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.6,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
  'aria-hidden': true,
}

export const IconeWhatsApp = ({ className }: P) => (
  <svg {...base} className={className}>
    <path d="M4.5 19.5l1.2-3.6A8 8 0 1 1 8.4 18.6z" />
    <path d="M9.2 8.6c.2-.5.5-.6.8-.6h.5c.2 0 .4.1.5.4l.6 1.4c.1.2 0 .5-.1.6l-.5.6c.5 1 1.3 1.8 2.3 2.3l.6-.5c.2-.1.4-.2.6-.1l1.4.6c.3.1.4.3.4.5v.5c0 .3-.1.6-.6.8-.6.3-1.8.3-3.3-.6a8.2 8.2 0 0 1-2.6-2.6c-.9-1.5-.9-2.7-.6-3.3z" fill="currentColor" stroke="none" />
  </svg>
)

export const IconeEmail = ({ className }: P) => (
  <svg {...base} className={className}>
    <rect x="3.5" y="5.5" width="17" height="13" rx="2" />
    <path d="M4 7l8 6 8-6" />
  </svg>
)

export const IconeDownload = ({ className }: P) => (
  <svg {...base} className={className}>
    <path d="M12 4v11M7.5 10.5 12 15l4.5-4.5M5 19.5h14" />
  </svg>
)

export const IconeSeta = ({ className }: P) => (
  <svg {...base} className={className}>
    <path d="M5 12h14M13 6l6 6-6 6" />
  </svg>
)

export const IconeVoltar = ({ className }: P) => (
  <svg {...base} className={className}>
    <path d="M15 6l-6 6 6 6" />
  </svg>
)

export const IconeAvancar = ({ className }: P) => (
  <svg {...base} className={className}>
    <path d="M9 6l6 6-6 6" />
  </svg>
)

export const IconeChave = ({ className }: P) => (
  <svg {...base} className={className}>
    <circle cx="8" cy="15" r="3.5" />
    <path d="M10.5 12.5 19 4m-3 3 2.5 2.5M14 9l2 2" />
  </svg>
)

export const IconeMais = ({ className }: P) => (
  <svg {...base} className={className}>
    <path d="M12 5v14M5 12h14" />
  </svg>
)

export const IconeFechar = ({ className }: P) => (
  <svg {...base} className={className}>
    <path d="M6 6l12 12M18 6 6 18" />
  </svg>
)

export const IconeSol = ({ className }: P) => (
  <svg {...base} className={className}>
    <circle cx="12" cy="12" r="4" />
    <path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6 7 7M17 17l1.4 1.4M5.6 18.4 7 17M17 7l1.4-1.4" />
  </svg>
)

export const IconeLua = ({ className }: P) => (
  <svg {...base} className={className}>
    <path d="M19.5 14.5A7.5 7.5 0 0 1 9.5 4.5a7.5 7.5 0 1 0 10 10z" />
  </svg>
)

export const IconeBusca = ({ className }: P) => (
  <svg {...base} className={className}>
    <circle cx="11" cy="11" r="6.5" />
    <path d="M16 16l4 4" />
  </svg>
)
