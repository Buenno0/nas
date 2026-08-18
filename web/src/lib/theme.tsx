import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type MouseEvent,
  type ReactNode,
} from 'react'
import { flushSync } from 'react-dom'

type Theme = 'dark' | 'light'

const STORAGE_KEY = 'nas-theme'
const DURACAO = 520

interface ThemeContextValue {
  theme: Theme
  /** Recebe o clique para a transição nascer no botão; sem ele, nasce no centro. */
  toggle: (evento?: MouseEvent) => void
}

const ThemeContext = createContext<ThemeContextValue>({ theme: 'dark', toggle: () => {} })

function currentTheme(): Theme {
  return document.documentElement.classList.contains('light') ? 'light' : 'dark'
}

function menosMovimento(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

/** Aplica o tema no documento. Fica fora do React porque a transição precisa
 *  que a troca aconteça de forma síncrona, dentro do callback da API. */
function aplicarNoDocumento(tema: Theme) {
  const root = document.documentElement
  root.classList.remove('dark', 'light')
  root.classList.add(tema)
  try {
    localStorage.setItem(STORAGE_KEY, tema)
  } catch {
    // navegação privada pode recusar; o tema só não fica salvo
  }
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute('content', tema === 'light' ? '#f6f7fa' : '#0a0b0e')
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  // O tema já foi aplicado pelo script inline do index.html; aqui só o lemos,
  // para o React não desfazer a escolha em um segundo render.
  const [theme, setTheme] = useState<Theme>(currentTheme)

  useEffect(() => {
    aplicarNoDocumento(theme)
  }, [theme])

  const toggle = useCallback((evento?: MouseEvent) => {
    const proximo: Theme = currentTheme() === 'dark' ? 'light' : 'dark'

    // O círculo nasce onde a pessoa clicou; sem evento (atalho, código), no meio.
    const x = evento?.clientX ?? window.innerWidth / 2
    const y = evento?.clientY ?? window.innerHeight / 2

    const suportaTransicao = typeof document.startViewTransition === 'function'
    if (!suportaTransicao || menosMovimento()) {
      // Sem a API, o CSS ainda dá um esmaecimento curto nas cores (ver
      // index.css); com movimento reduzido, a troca é seca de propósito.
      document.documentElement.dataset.temaTrocando = 'sim'
      window.setTimeout(() => delete document.documentElement.dataset.temaTrocando, DURACAO)
      setTheme(proximo)
      return
    }

    const transicao = document.startViewTransition(() => {
      // flushSync garante que a nova árvore (marca, ícones) já esteja pintada
      // quando o navegador tira o retrato do estado final.
      flushSync(() => {
        aplicarNoDocumento(proximo)
        setTheme(proximo)
      })
    })

    transicao.ready
      .then(() => {
        // Raio até o canto mais distante, para o círculo cobrir a tela inteira.
        const raio = Math.hypot(
          Math.max(x, window.innerWidth - x),
          Math.max(y, window.innerHeight - y),
        )
        document.documentElement.animate(
          {
            clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${raio}px at ${x}px ${y}px)`],
          },
          {
            duration: DURACAO,
            easing: 'cubic-bezier(0.4, 0, 0.2, 1)',
            pseudoElement: '::view-transition-new(root)',
          },
        )
      })
      .catch(() => {
        // A API aborta quando a aba está oculta ou quando outra transição
        // entra no meio. O tema já trocou dentro do callback, então aqui só
        // resta o consolo do esmaecimento de cores — e nenhuma promessa
        // rejeitada solta no console.
        document.documentElement.dataset.temaTrocando = 'sim'
        window.setTimeout(() => delete document.documentElement.dataset.temaTrocando, DURACAO)
      })
  }, [])

  return <ThemeContext.Provider value={{ theme, toggle }}>{children}</ThemeContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useTheme() {
  return useContext(ThemeContext)
}
