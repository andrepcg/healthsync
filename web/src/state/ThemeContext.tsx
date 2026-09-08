import { createContext, useContext, type ReactNode } from 'react'
import { useTheme, type Theme } from './theme'

interface Ctx {
  theme: Theme
  resolved: 'light' | 'dark'
  setTheme: (t: Theme) => void
}

const ThemeCtx = createContext<Ctx>({ theme: 'system', resolved: 'light', setTheme: () => {} })

export function ThemeProvider({ children }: { children: ReactNode }) {
  const v = useTheme()
  return <ThemeCtx.Provider value={v}>{children}</ThemeCtx.Provider>
}

export const useThemeCtx = () => useContext(ThemeCtx)
export const useThemeMode = () => useContext(ThemeCtx).resolved
