import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'

type Theme = 'dark' | 'light'

const STORAGE_KEY = 'nas-theme'

interface ThemeContextValue {
  theme: Theme
  toggle: () => void
}

const ThemeContext = createContext<ThemeContextValue>({ theme: 'dark', toggle: () => {} })

function currentTheme(): Theme {
  return document.documentElement.classList.contains('light') ? 'light' : 'dark'
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  // O tema já foi aplicado pelo script inline do index.html; aqui só o lemos,
  // para o React não desfazer a escolha em um segundo render.
  const [theme, setTheme] = useState<Theme>(currentTheme)

  useEffect(() => {
    const root = document.documentElement
    root.classList.remove('dark', 'light')
    root.classList.add(theme)
    localStorage.setItem(STORAGE_KEY, theme)
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute('content', theme === 'light' ? '#f6f7fa' : '#0a0b0e')
  }, [theme])

  const toggle = useCallback(() => setTheme((t) => (t === 'dark' ? 'light' : 'dark')), [])

  return <ThemeContext.Provider value={{ theme, toggle }}>{children}</ThemeContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useTheme() {
  return useContext(ThemeContext)
}
