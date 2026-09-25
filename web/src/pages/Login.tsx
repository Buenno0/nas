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
            <Ceu />
            <Cadente />
            <Lua />
            <Dunas />
          </div>

          <div className="lg-mark">
            {/* A marca do app, não um selo próprio: assim ela acompanha o tema
                — ouro no escuro, pedra e ouro no claro — em vez de ficar um
                selo solto em cima da areia. */}
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
                    stroke="currentColor"
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

/** Estrelas. Cada uma cintila no seu próprio ritmo — duração e atraso
 *  distintos, senão o céu inteiro pisca junto e parece um LED.
 *
 *  Coordenadas no viewBox 0 0 800 400, ancorado no topo. `cy` para em 232
 *  porque abaixo disso são as dunas: estrela em cima de areia é erro de
 *  desenho, não de opacidade. */
const estrelas = [
  { cx: 38, cy: 52, r: 1.1, duracao: '6.5s', atraso: '-0.4s' },
  { cx: 96, cy: 118, r: 0.8, duracao: '8.0s', atraso: '-3.1s' },
  { cx: 134, cy: 30, r: 1.4, duracao: '5.5s', atraso: '-1.9s' },
  { cx: 168, cy: 176, r: 0.9, duracao: '9.0s', atraso: '-5.6s' },
  { cx: 212, cy: 76, r: 1.2, duracao: '7.0s', atraso: '-2.3s' },
  { cx: 247, cy: 142, r: 0.7, duracao: '6.0s', atraso: '-4.8s' },
  { cx: 286, cy: 24, r: 1.0, duracao: '8.5s', atraso: '-0.9s' },
  { cx: 318, cy: 196, r: 1.3, duracao: '5.0s', atraso: '-6.2s' },
  { cx: 352, cy: 96, r: 0.8, duracao: '7.5s', atraso: '-3.7s' },
  { cx: 391, cy: 46, r: 1.5, duracao: '6.2s', atraso: '-1.2s' },
  { cx: 424, cy: 158, r: 1.0, duracao: '9.5s', atraso: '-5.1s' },
  { cx: 462, cy: 108, r: 0.7, duracao: '5.8s', atraso: '-2.6s' },
  { cx: 498, cy: 62, r: 1.2, duracao: '8.2s', atraso: '-4.3s' },
  { cx: 531, cy: 208, r: 0.9, duracao: '6.8s', atraso: '-0.6s' },
  { cx: 569, cy: 34, r: 1.1, duracao: '7.8s', atraso: '-6.9s' },
  { cx: 604, cy: 128, r: 1.4, duracao: '5.3s', atraso: '-2.0s' },
  { cx: 641, cy: 88, r: 0.8, duracao: '9.2s', atraso: '-4.6s' },
  { cx: 676, cy: 182, r: 1.0, duracao: '6.6s', atraso: '-1.5s' },
  { cx: 712, cy: 56, r: 1.3, duracao: '8.8s', atraso: '-3.4s' },
  { cx: 748, cy: 146, r: 0.9, duracao: '5.6s', atraso: '-5.9s' },
  { cx: 778, cy: 98, r: 1.1, duracao: '7.2s', atraso: '-2.8s' },
  { cx: 62, cy: 172, r: 1.0, duracao: '8.6s', atraso: '-6.4s' },
  { cx: 270, cy: 116, r: 0.9, duracao: '6.4s', atraso: '-1.7s' },
  { cx: 452, cy: 226, r: 0.8, duracao: '7.6s', atraso: '-4.1s' },
  { cx: 588, cy: 152, r: 0.7, duracao: '9.8s', atraso: '-0.2s' },
  { cx: 156, cy: 100, r: 1.2, duracao: '6.9s', atraso: '-5.3s' },
]

/**
 * A lua da cena — só no tema escuro.
 *
 * Ela não é enfeite: é a fonte de luz que faltava. A cena escura tinha uma
 * estátua com a touca dourada e nada explicando de onde vinha o ouro. O CSS
 * a esconde no claro (lá o sol é o degradê `--lg-glow`); o SVG fica no DOM
 * nos dois casos, porque `display: none` é mais barato que remontar a árvore
 * a cada troca de tema — e a troca acontece dentro de uma View Transition.
 *
 * Gibosa minguante: o disco é comido por um segundo círculo deslocado. Meia
 * lua exata ficaria geométrica demais ao lado de uma estátua gasta.
 */
/**
 * O campo de estrelas. Só no escuro, como a lua.
 *
 * Fica atrás da lua e das dunas na ordem de pintura, que é a ordem do DOM:
 * o halo lava as estrelas que ficam perto, e a areia cobre as de baixo. Isso
 * poupa recortar o campo à mão.
 */
function Ceu() {
  return (
    <svg
      className="lg-ceu"
      viewBox="0 0 800 400"
      preserveAspectRatio="xMidYMin slice"
      aria-hidden="true"
    >
      {estrelas.map((e) => (
        <circle
          key={`${e.cx}-${e.cy}`}
          className="lg-estrela"
          cx={e.cx}
          cy={e.cy}
          r={e.r}
          fill="var(--lg-estrela)"
          style={{ animationDuration: e.duracao, animationDelay: e.atraso }}
        />
      ))}
    </svg>
  )
}

/**
 * A estrela cadente, num elemento próprio.
 *
 * Ela não vive dentro do campo de estrelas de propósito. O campo usa
 * `preserveAspectRatio="slice"`, que recorta as laterais quando o contêiner é
 * mais estreito que o viewBox — e a coluna do login é. Um risco ancorado em
 * coordenada de viewBox saía de cena na metade do caminho. Aqui a caixa é
 * posicionada em porcentagem, como a lua, e o trajeto inteiro cabe dentro
 * dela em qualquer largura.
 */
function Cadente() {
  return (
    <svg className="lg-cadente-caixa" viewBox="0 0 160 100" aria-hidden="true">
      <defs>
        {/* O rastro acende na direção do movimento: transparente na cauda,
            cheio na cabeça. */}
        <linearGradient id="lg-rastro" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="var(--lg-estrela)" stopOpacity="0" />
          <stop offset="100%" stopColor="var(--lg-estrela)" stopOpacity="0.9" />
        </linearGradient>
      </defs>
      <g className="lg-cadente">
        <line
          x1="2"
          y1="2"
          x2="52"
          y2="33"
          stroke="url(#lg-rastro)"
          strokeWidth="1.5"
          strokeLinecap="round"
        />
        <circle cx="52" cy="33" r="1.7" fill="var(--lg-estrela)" />
      </g>
    </svg>
  )
}

function Lua() {
  return (
    <div className="lg-lua-caixa">
      {/* O alvo do ponteiro é este <span>, não o SVG.
 
          O disco é um <g> dentro do SVG, e `:hover` sobre grupo SVG não
          invalida estilo direito neste motor: o sinalizador vira, o estilo
          computado fica um estado atrás — apaga ao sair e acende ao entrar.
          Nas telas de erro o defeito se escondia, porque o parallax escreve
          transform inline a cada movimento e força o recálculo.
 
          Um elemento HTML não tem esse problema. `inset: 27.5%` recorta
          exatamente o disco: ele é r=27 num viewBox de 120, ou 45% da caixa,
          centrado. O halo fica de fora do alvo de propósito — encostar no
          brilho não é encostar na lua. */}
      <span className="lg-lua-alvo" aria-hidden="true" />
      <svg className="lg-lua" viewBox="0 0 120 120" aria-hidden="true">
        <defs>
          <radialGradient id="lg-lua-halo">
            <stop offset="34%" stopColor="var(--lg-lua-halo)" stopOpacity="0.4" />
            <stop offset="62%" stopColor="var(--lg-lua-halo)" stopOpacity="0.11" />
            <stop offset="100%" stopColor="var(--lg-lua-halo)" stopOpacity="0" />
          </radialGradient>
          {/* Branco é o que fica; preto é o que a sombra come.
              A fase se constrói em três passos, e a ordem importa: o disco
              inteiro, o meio da direita apagado, e uma elipse acesa de volta.
              É essa elipse que faz o terminador arquear para o lado escuro —
              um segundo círculo mordendo a borda arqueia para o lado errado e
              produz uma foice, não uma gibosa. rx=12 sobre ry=27 dá 72% de
              face iluminada. */}
          <mask id="lg-lua-gibosa">
            <rect width="120" height="120" fill="#000" />
            <circle cx="60" cy="60" r="27" fill="#fff" />
            <rect x="60" y="0" width="60" height="120" fill="#000" />
            <ellipse cx="60" cy="60" rx="12" ry="27" fill="#fff" />
          </mask>
        </defs>

        {/* Uma camada por dono: as duas animam `transform` e, empilhadas no
            mesmo elemento, a última declarada apagaria a outra. Nas telas de
            erro há ainda uma terceira camada — o próprio <svg>, que recebe o
            parallax por style inline. */}
        <g className="lg-lua-corpo">
          <g className="lg-lua-deriva">
            <circle className="lg-lua-halo" cx="60" cy="60" r="58" fill="url(#lg-lua-halo)" />
            <g mask="url(#lg-lua-gibosa)">
              <circle cx="60" cy="60" r="27" fill="var(--lg-lua)" />
              {/* As crateras repetem as órbitas vazadas da máscara: a mesma
                  pedra gasta, na mesma paleta. */}
              <circle cx="49" cy="51" r="5.5" fill="var(--lg-lua-cratera)" />
              <circle cx="43" cy="67" r="3.4" fill="var(--lg-lua-cratera)" />
              <circle cx="54" cy="72" r="2.2" fill="var(--lg-lua-cratera)" />
              <circle cx="40" cy="55" r="2" fill="var(--lg-lua-cratera)" />
            </g>
          </g>
        </g>
      </svg>
    </div>
  )
}

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
