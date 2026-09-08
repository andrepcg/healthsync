import { useEffect, useState } from 'react'

export type Theme = 'light' | 'dark' | 'system'

const KEY = 'healthsync.theme'

function read(): Theme {
  try {
    const v = localStorage.getItem(KEY)
    if (v === 'light' || v === 'dark') return v
  } catch {
    /* ignore */
  }
  return 'system'
}

export function resolveTheme(t: Theme): 'light' | 'dark' {
  if (t !== 'system') return t
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(read)
  const [resolved, setResolved] = useState(() => resolveTheme(theme))

  useEffect(() => {
    const apply = () => {
      const r = resolveTheme(theme)
      setResolved(r)
      document.documentElement.dataset.theme = r
    }
    apply()
    const mq = window.matchMedia?.('(prefers-color-scheme: dark)')
    mq?.addEventListener('change', apply)
    return () => mq?.removeEventListener('change', apply)
  }, [theme])

  const set = (t: Theme) => {
    try {
      if (t === 'system') localStorage.removeItem(KEY)
      else localStorage.setItem(KEY, t)
    } catch {
      /* ignore */
    }
    setTheme(t)
  }
  return { theme, resolved, setTheme: set }
}
