import { useState, type FormEvent, type KeyboardEvent } from 'react'
import { Link } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useTheme } from '../lib/theme'
import { MoonIcon, SunIcon } from '../components/icons'
import { Mark } from '../components/Mark'
import './login.css'

export function Login() {
  const queryClient = useQueryClient()
  const { theme, toggle } = useTheme()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [remember, setRemember] = useState(true)
  const [verSenha, setVerSenha] = useState(false)
  const [capsLock, setCapsLock] = useState(false)
  const [comoRecuperar, setComoRecuperar] = useState(false)
  const [erroLocal, setErroLocal] = useState('')
  const [tremer, setTremer] = useState(false)

  const login = useMutation({
    mutationFn: () => api.login(username.trim(), password, remember),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['me'] }),
    onError: () => sacudir(),
  })

  // O tremor precisa reiniciar a cada erro, então a classe sai e volta.
  function sacudir() {
    setTremer(false)
    requestAnimationFrame(() => setTremer(true))
  }

  const submeter = (e: FormEvent) => {
    e.preventDefault()
    setErroLocal('')
    if (!username.trim() || !password) {
      setErroLocal('Preencha usuário e senha.')
      sacudir()
      return
    }
    login.mutate()
  }

  // Caps Lock ligado com senha oculta é a causa nº 1 de "minha senha não
  // funciona"; avisar é mais gentil que deixar errar.
  const olharCapsLock = (e: KeyboardEvent<HTMLInputElement>) => {
    if (typeof e.getModifierState === 'function') {
      setCapsLock(e.getModifierState('CapsLock'))
    }
  }

  const erro = erroLocal || (login.isError ? (login.error as Error).message : '')

  return (
    <div className="tela-login">
      <button
        type="button"
        className="lg-theme"
        onClick={(e) => toggle(e)}
        aria-label={theme === 'dark' ? 'Mudar para o tema claro' : 'Mudar para o tema escuro'}
      >
        {theme === 'dark' ? <SunIcon width="17" height="17" /> : <MoonIcon width="17" height="17" />}
      </button>

      <div className="lg-shell">
        <section className="lg-stage">
          <div className="lg-dunes-clip">
            <Dunas />
          </div>

          <div className="lg-mark">
            {/* A marca do app, não um selo próprio: assim ela acompanha o tema
                — violeta no escuro, terracota no claro — em vez de ficar um
                quadrado violeta em cima da areia. */}
            <Mark size={42} />
            <div>
              <div className="lg-name">Ozymandias</div>
              <div className="lg-sub">n a s</div>
            </div>
          </div>

          <div className="lg-pitch">
            <h1>
              Seu acervo,
              <br />
              na sua rede.
            </h1>
            <blockquote>
              “Contemplai minha obra, ó Poderosos, e desesperai!”
              <cite>Shelley, 1818</cite>
            </blockquote>
          </div>
        </section>

        <section className="lg-pane">
          <form className={`lg-form${tremer ? ' lg-shake' : ''}`} onSubmit={submeter} noValidate>
            <div className="lg-head">
              <h2>Entrar</h2>
              <p>Use sua conta do servidor.</p>
            </div>

            {erro && (
              <div className="lg-hint lg-err" role="alert">
                <svg
                  width="15"
                  height="15"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  aria-hidden="true"
                >
                  <circle cx="12" cy="12" r="9" />
                  <path d="M12 8v4M12 16h.01" />
                </svg>
                <span>{erro}</span>
              </div>
            )}

            <div className="lg-field">
              <label htmlFor="usuario">Usuário</label>
              <div className="lg-wrap">
                <svg
                  className="lg-lead"
                  width="17"
                  height="17"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                  aria-hidden="true"
                >
                  <circle cx="12" cy="8" r="3.6" />
                  <path d="M5 20c1.2-3.4 4-5 7-5s5.8 1.6 7 5" />
                </svg>
                <input
                  id="usuario"
                  name="username"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="seu.usuario"
                  autoComplete="username"
                  autoCapitalize="none"
                  spellCheck={false}
                  autoFocus
                />
              </div>
            </div>

            <div className="lg-field">
              <label htmlFor="senha">Senha</label>
              <div className="lg-wrap">
                <svg
                  className="lg-lead"
                  width="17"
                  height="17"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                  aria-hidden="true"
                >
                  <rect x="4.5" y="10.5" width="15" height="9.5" rx="2.5" />
                  <path d="M8.5 10.5V8a3.5 3.5 0 0 1 7 0v2.5" />
                </svg>
                <input
                  id="senha"
                  name="password"
                  type={verSenha ? 'text' : 'password'}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  onKeyDown={olharCapsLock}
                  onKeyUp={olharCapsLock}
                  onBlur={() => setCapsLock(false)}
                  placeholder="••••••••"
                  autoComplete="current-password"
                />
                <button
                  type="button"
                  className="lg-peek"
                  onClick={() => setVerSenha((v) => !v)}
                  aria-label={verSenha ? 'Ocultar senha' : 'Mostrar senha'}
                >
                  {verSenha ? <OlhoFechado /> : <OlhoAberto />}
                </button>
              </div>
            </div>

            {capsLock && (
              <div className="lg-hint">
                <svg
                  width="15"
                  height="15"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                  aria-hidden="true"
                >
                  <path d="M12 4l7 7h-4v4H9v-4H5l7-7zM8 19h8" />
                </svg>
                <span>Caps Lock está ativo.</span>
              </div>
            )}

            <div className="lg-row">
              <label className="lg-check">
                <input
                  type="checkbox"
                  checked={remember}
                  onChange={(e) => setRemember(e.target.checked)}
                />
                <span className="lg-box">
                  <svg
                    width="11"
                    height="11"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="#fff"
                    strokeWidth="3.4"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                  >
                    <path d="M5 12.5l4.5 4.5L19 7.5" />
                  </svg>
                </span>
                Manter conectado
              </label>
              <button
                type="button"
                className="lg-link"
                onClick={() => setComoRecuperar((v) => !v)}
                aria-expanded={comoRecuperar}
              >
                Esqueci a senha
              </button>
            </div>

            {/* Não existe recuperação por e-mail: o servidor é seu, não há de
                onde mandar mensagem. Quem tem o terminal redefine a senha. */}
            {comoRecuperar && (
              <div className="lg-hint">
                <span>
                  Quem administra o servidor redefine no terminal, com{' '}
                  <code>nas passwd {username.trim() || 'seu.usuario'}</code>.
                </span>
              </div>
            )}

            <button className="lg-submit" type="submit" disabled={login.isPending}>
              {login.isPending ? (
                <>
                  <svg
                    className="lg-spin"
                    width="16"
                    height="16"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2.4"
                    strokeLinecap="round"
                    aria-hidden="true"
                  >
                    <path d="M12 3a9 9 0 1 0 9 9" />
                  </svg>
                  Entrando…
                </>
              ) : (
                // Sem seta: ela não informava nada que a palavra já não diga.
                'Entrar'
              )}
            </button>

            <div className="lg-foot">
              <div className="lg-sep">ou</div>
              {/* Não há conta de convidado; as ruínas são o único lugar
                  público do servidor. */}
              <Link className="lg-ghost" to="/ruinas">
                <svg
                  width="15"
                  height="15"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                  aria-hidden="true"
                >
                  <path d="M12 4 2.8 20h18.4L12 4Z" />
                  <path d="M12 10v4M12 17.2v.1" />
                </svg>
                Visitar as ruínas
              </Link>
              <div className="lg-meta">
                <span className="lg-dot" /> servidor online · {window.location.host}
              </div>
            </div>
          </form>
        </section>
      </div>
    </div>
  )
}


/** Poeira que sobe pelas dunas. Cada grão tem duração e atraso próprios para
 *  o movimento não ficar sincronizado. */
const graos = [
  { cx: 70, cy: 196, r: 1.8, duracao: '17s', atraso: '-1s' },
  { cx: 182, cy: 212, r: 1.3, duracao: '21s', atraso: '-5s' },
  { cx: 296, cy: 188, r: 2.2, duracao: '15s', atraso: '-9s' },
  { cx: 408, cy: 220, r: 1.5, duracao: '24s', atraso: '-3s' },
  { cx: 520, cy: 196, r: 1.9, duracao: '19s', atraso: '-13s' },
  { cx: 648, cy: 210, r: 1.3, duracao: '16s', atraso: '-7s' },
  { cx: 762, cy: 192, r: 2, duracao: '22s', atraso: '-11s' },
]

function Dunas() {
  return (
    <svg
      className="lg-dunes"
      viewBox="0 0 820 260"
      preserveAspectRatio="xMaxYMax slice"
      aria-hidden="true"
    >
      <g className="lg-haze">
        <path
          d="M0 96 C120 66 200 114 320 104 C450 94 520 56 640 72 C720 82 780 104 820 96 L820 260 L0 260 Z"
          fill="var(--lg-dune-0)"
        />
        <path
          d="M820 96 C940 66 1020 114 1140 104 C1270 94 1340 56 1460 72 C1540 82 1600 104 1640 96 L1640 260 L820 260 Z"
          fill="var(--lg-dune-0)"
        />
      </g>

      <g className="lg-mask">
        <g transform="translate(632 44) scale(1.32)">
          <path
            d="M18 22 C18 14 27 9 48 9 C69 9 78 14 78 22 L78 52 C78 68 66 80 48 87 C30 80 18 68 18 52 Z"
            fill="var(--lg-stone)"
          />
          <path
            className="lg-nemes"
            d="M24 24 C24 18 32 14 48 14 C64 14 72 18 72 24 L72 34 L24 34 Z"
            fill="var(--lg-stone-2)"
          />
          <rect className="lg-nemes" x="24" y="36" width="48" height="4" fill="var(--lg-stone-2)" />
          <rect x="29" y="50" width="16" height="5" rx="2.5" fill="var(--lg-cut)" />
          <rect x="51" y="50" width="16" height="5" rx="2.5" fill="var(--lg-cut)" />
          <path d="M41 66 L41 78 L55 72 Z" fill="var(--lg-cut)" />
        </g>
      </g>

      <g className="lg-layer-1">
        <path
          d="M0 158 C140 130 240 178 380 168 C520 158 600 120 740 140 C780 146 800 156 820 152 L820 260 L0 260 Z"
          fill="var(--lg-dune-1)"
        />
      </g>

      {graos.map((g) => (
        <circle
          key={`${g.cx}-${g.cy}`}
          className="lg-mote"
          cx={g.cx}
          cy={g.cy}
          r={g.r}
          fill="var(--lg-mote)"
          style={{ animationDuration: g.duracao, animationDelay: g.atraso }}
        />
      ))}

      <g className="lg-layer-2">
        <path
          d="M0 202 C160 182 300 212 460 206 C620 200 720 174 820 192 L820 260 L0 260 Z"
          fill="var(--lg-dune-2)"
        />
      </g>

      <g className="lg-drift">
        <path
          d="M0 226 C120 216 220 236 340 232 C460 228 540 214 660 224 C740 230 780 236 820 233 L820 260 L0 260 Z"
          fill="var(--lg-dune-3)"
        />
        <path
          d="M820 226 C940 216 1040 236 1160 232 C1280 228 1360 214 1480 224 C1560 230 1600 236 1640 233 L1640 260 L820 260 Z"
          fill="var(--lg-dune-3)"
        />
      </g>
    </svg>
  )
}

function OlhoAberto() {
  return (
    <svg
      width="17"
      height="17"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      aria-hidden="true"
    >
      <path d="M2.5 12S6 5.8 12 5.8 21.5 12 21.5 12 18 18.2 12 18.2 2.5 12 2.5 12z" />
      <circle cx="12" cy="12" r="2.8" />
    </svg>
  )
}

function OlhoFechado() {
  return (
    <svg
      width="17"
      height="17"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      aria-hidden="true"
    >
      <path d="M4 4l16 16" />
      <path d="M9.6 6.3A9.6 9.6 0 0 1 12 6c6 0 9.5 6 9.5 6a17 17 0 0 1-2.7 3.4M6.4 8.2A17 17 0 0 0 2.5 12S6 18 12 18a9 9 0 0 0 3-.5" />
    </svg>
  )
}
