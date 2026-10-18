// A Estante: o PS5 e a TV da sala, vistos pela rede do Mac. Feita para o
// celular no sofá — botões grandes, nada que precise de mira.
import { useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type EstadoEstante } from '../lib/api'
import { guardarEstante, useEstante } from '../lib/useEstante'
import { EmptyState, ErrorState, Spinner } from '../components/states'
import { GamepadIcon, MinusIcon, PlusIcon, TvIcon, VolumeIcon } from '../components/icons'

function Cartao({ icone, titulo, children }: { icone: React.ReactNode; titulo: string; children: React.ReactNode }) {
  return (
    <section className="rounded-xl border border-line bg-surface p-5">
      <div className="flex items-center gap-3">
        <span className="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-elev text-xl text-muted">{icone}</span>
        <h2 className="min-w-0 flex-1 truncate text-base font-semibold">{titulo}</h2>
      </div>
      <div className="mt-4">{children}</div>
    </section>
  )
}

function Pilula({ ativo, children }: { ativo: boolean; children: React.ReactNode }) {
  return (
    <span
      className={`inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold ${
        ativo ? 'border-ok/40 bg-ok/10 text-ok' : 'border-line text-muted'
      }`}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${ativo ? 'bg-ok' : 'bg-muted/60'}`} aria-hidden="true" />
      {children}
    </span>
  )
}

function CartaoPS5({ ps5 }: { ps5: EstadoEstante['ps5'] }) {
  const ligado = ps5?.estado === 'ligado'
  return (
    <Cartao icone={<GamepadIcon />} titulo={ps5?.nome ?? 'PS5'}>
      <div className="flex items-center justify-between gap-3">
        <p className="min-w-0 text-sm text-muted">
          {!ps5
            ? 'Desligado, ou em descanso sem atender a rede.'
            : ligado
              ? ps5.jogo
                ? <>Jogando <span className="font-medium text-ink">{ps5.jogo}</span></>
                : 'Nenhum jogo aberto.'
              : 'Em descanso.'}
        </p>
        <Pilula ativo={ligado}>{!ps5 ? 'Desligado' : ligado ? 'Ligado' : 'Em descanso'}</Pilula>
      </div>
    </Cartao>
  )
}

function CartaoTV({ tv }: { tv: EstadoEstante['tv'] }) {
  const queryClient = useQueryClient()
  const [erro, setErro] = useState<string>()
  // Valor do slider enquanto o dedo arrasta; null = mostra o que a TV disse.
  const [arrasto, setArrasto] = useState<number | null>(null)
  const pendente = useRef<number | null>(null)
  const enviando = useRef(false)

  const aoResponder = {
    onSuccess: (e: EstadoEstante) => {
      guardarEstante(queryClient, e)
      setErro(undefined)
    },
    onError: (e: unknown) => setErro(e instanceof Error ? e.message : 'A TV não respondeu.'),
  }
  const passo = useMutation({ mutationFn: (d: number) => api.volumeDaTV({ passo: d }), ...aoResponder })
  const mudo = useMutation({ mutationFn: api.mudoDaTV, ...aoResponder })

  // Uma chamada por vez: enquanto uma está no ar, só o último valor do slider
  // espera a vez. Arrastar não vira uma rajada de pedidos para a TV.
  const arrastar = (v: number) => {
    setArrasto(v)
    pendente.current = v
    if (enviando.current) return
    enviando.current = true
    void (async () => {
      while (pendente.current !== null) {
        const valor = pendente.current
        pendente.current = null
        try {
          aoResponder.onSuccess(await api.volumeDaTV({ valor }))
        } catch (e) {
          aoResponder.onError(e)
        }
      }
      enviando.current = false
      setArrasto(null)
    })()
  }

  if (!tv) {
    return (
      <Cartao icone={<TvIcon />} titulo="TV">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm text-muted">Ela só aparece na rede quando está ligada.</p>
          <Pilula ativo={false}>Desligada</Pilula>
        </div>
      </Cartao>
    )
  }

  const volume = arrasto ?? tv.volume
  const ocupado = passo.isPending || mudo.isPending
  const botao =
    'grid h-12 w-12 shrink-0 place-items-center rounded-full border border-line bg-elev text-xl transition hover:border-accent/50 active:scale-95 disabled:opacity-50'

  return (
    <Cartao icone={<TvIcon />} titulo={tv.nome}>
      <div className="flex items-end justify-between gap-3">
        <div>
          <p className="rotulo">Volume</p>
          <p className={`text-5xl font-semibold tabular-nums tracking-tight ${tv.mudo ? 'text-muted line-through decoration-2' : ''}`}>
            {volume}
          </p>
        </div>
        <Pilula ativo>{tv.mudo ? 'Mudo' : 'Ligada'}</Pilula>
      </div>

      <div className="mt-5 flex items-center gap-3">
        <button type="button" className={botao} disabled={ocupado || volume <= 0} onClick={() => passo.mutate(-1)} aria-label="Baixar o volume">
          <MinusIcon />
        </button>
        <input
          type="range"
          min={0}
          max={tv.max}
          step={1}
          value={volume}
          onChange={(e) => arrastar(Number(e.target.value))}
          aria-label="Volume da TV"
          className="h-2 min-w-0 flex-1 cursor-pointer appearance-none rounded-full bg-line
                     [&::-webkit-slider-thumb]:h-6 [&::-webkit-slider-thumb]:w-6 [&::-webkit-slider-thumb]:appearance-none
                     [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-accent
                     [&::-moz-range-thumb]:h-6 [&::-moz-range-thumb]:w-6 [&::-moz-range-thumb]:rounded-full
                     [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:bg-accent"
          style={{ background: `linear-gradient(to right, var(--accent) ${(volume / tv.max) * 100}%, var(--line) 0)` }}
        />
        <button type="button" className={botao} disabled={ocupado || volume >= tv.max} onClick={() => passo.mutate(1)} aria-label="Subir o volume">
          <PlusIcon />
        </button>
      </div>

      <button
        type="button"
        disabled={ocupado}
        onClick={() => mudo.mutate(!tv.mudo)}
        aria-pressed={tv.mudo}
        className={`mt-4 flex h-12 w-full items-center justify-center gap-2 rounded-lg border text-sm font-semibold transition active:scale-[0.99] disabled:opacity-50 ${
          tv.mudo ? 'border-accent bg-accent text-accent-ink' : 'border-line bg-elev hover:border-accent/50'
        }`}
      >
        <VolumeIcon muted={!tv.mudo} /> {tv.mudo ? 'Tirar do mudo' : 'Mudo'}
      </button>

      {erro && (
        <p role="alert" className="mt-3 text-sm text-danger">
          {erro}
        </p>
      )}
    </Cartao>
  )
}

export function Estante() {
  const { data, error, isLoading, refetch } = useEstante()

  if (isLoading) return <Spinner label="Procurando o PS5 e a TV" />
  if (error) return <ErrorState error={error} retry={() => void refetch()} />
  if (!data?.disponivel) {
    return (
      <EmptyState
        title="A Estante só funciona em casa"
        description="Ela fala com o PS5 e a TV pela rede do Mac. Esta é a instância da nuvem, que não enxerga a sala."
      />
    )
  }

  return (
    <div className="mx-auto max-w-xl space-y-4 px-4 py-6 sm:px-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight sm:text-2xl">Estante</h1>
        <p className="mt-1 text-sm text-muted">O PS5 e a TV da sala, vistos pela rede do Mac.</p>
      </div>
      <CartaoPS5 ps5={data.ps5} />
      <CartaoTV tv={data.tv} />
    </div>
  )
}
