import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { Mark } from '../components/Mark'
import { ChevronRight, WarningIcon } from '../components/icons'

interface Placar {
  por_codigo: Record<string, number>
  total: number
}

// As iscas. Cada uma é uma navegação de verdade para uma rota que devolve o
// código de status correspondente — não é simulação: o servidor responde 401,
// 404 ou 500 mesmo, e o navegador mostra a tela daquele erro.
const iscas = [
  {
    codigo: 401,
    numero: '01',
    titulo: 'Entrada proibida',
    chamada: 'Abra a passagem selada',
    detalhe: 'O guardião exige uma identidade antes de deixar você passar.',
    cor: 'from-accent/20',
  },
  {
    codigo: 404,
    numero: '02',
    titulo: 'Caminho perdido',
    chamada: 'Siga a rota que não existe',
    detalhe: 'Não há nada no destino, além de areia e uma estátua esquecida.',
    cor: 'from-warn/20',
  },
  {
    codigo: 500,
    numero: '03',
    titulo: 'Colapso final',
    chamada: 'Veja o servidor desabar',
    detalhe: 'A obra inteira ruiu. É um desastre controlado: o acervo continua intacto.',
    cor: 'from-danger/20',
  },
] as const

/**
 * Rota interna desconhecida: manda o navegador buscar a página de 404 no
 * servidor. Precisa ser navegação de verdade (location.replace) — um <Navigate>
 * do React voltaria a cair aqui e giraria em círculos, além de nunca produzir
 * um status 404 real.
 */
export function NaoEncontrado() {
  useEffect(() => {
    window.location.replace('/ruinas/404')
  }, [])
  return null
}

export function Ruinas() {
  const { data } = useQuery<Placar>({
    queryKey: ['ruinas'],
    queryFn: async () => (await fetch('/api/ruinas')).json(),
    refetchOnMount: 'always',
  })

  return (
    <div className="mx-auto max-w-5xl px-4 py-8 sm:px-6 sm:py-12">
      <header className="relative mb-10 overflow-hidden rounded-2xl border border-line bg-surface px-6 py-9 sm:px-10 sm:py-12">
        <div
          aria-hidden
          className="pointer-events-none absolute -top-32 -right-20 h-80 w-80 rounded-full bg-accent opacity-15 blur-3xl"
        />
        <div className="relative flex items-start gap-4">
          <Mark size={48} />
          <div>
            <p className="mb-3 font-mono text-[11px] font-semibold tracking-[0.24em] text-accent uppercase">
              Arquivo de falhas
            </p>
            <h1 className="max-w-2xl text-4xl leading-[0.98] font-bold tracking-[-0.045em] sm:text-6xl">
              Três portas.{' '}
              <span className="text-accent">Escolha sua ruína.</span>
            </h1>
            <p className="mt-5 max-w-xl text-sm leading-relaxed text-muted sm:text-base">
              Cada opção abre uma tela de erro real. O código muda, o cenário desaba e seu
              acervo permanece exatamente onde estava.
            </p>
          </div>
        </div>
      </header>

      <section aria-labelledby="menu-ruinas">
        <div className="mb-4 flex items-end justify-between gap-4">
          <div>
            <p className="font-mono text-[10px] tracking-[0.2em] text-muted uppercase">Menu</p>
            <h2 id="menu-ruinas" className="mt-1 text-lg font-semibold tracking-tight">
              Qual erro você quer encontrar?
            </h2>
          </div>
          <p className="hidden text-xs text-muted sm:block">3 opções</p>
        </div>

        <nav aria-label="Telas de erro">
          <ul className="grid gap-3 sm:grid-cols-3">
            {iscas.map((isca) => (
              <li key={isca.codigo}>
                {/* Link comum, não navegação do React: a página tem que sair do
                    SPA para o navegador receber o status de verdade. */}
                <a
                  href={`/ruinas/${isca.codigo}`}
                  className="group relative flex h-full min-h-64 flex-col overflow-hidden rounded-xl border border-line bg-surface p-5 transition duration-200 hover:-translate-y-1 hover:border-accent hover:shadow-xl hover:shadow-black/15"
                >
                  <span
                    aria-hidden
                    className={`pointer-events-none absolute inset-0 bg-gradient-to-b ${isca.cor} to-transparent opacity-40 transition group-hover:opacity-100`}
                  />
                  <span className="relative flex items-center justify-between font-mono text-[10px] tracking-[0.18em] text-muted uppercase">
                    Opção {isca.numero}
                    <ChevronRight className="text-accent transition-transform group-hover:translate-x-1" />
                  </span>
                  <span className="relative mt-7 font-mono text-5xl font-semibold tracking-[-0.06em]">
                    {isca.codigo}
                  </span>
                  <span className="relative mt-5 block text-xs font-semibold tracking-wider text-accent uppercase">
                    {isca.titulo}
                  </span>
                  <span className="relative mt-1 block text-base font-semibold leading-snug">
                    {isca.chamada}
                  </span>
                  <span className="relative mt-2 block text-xs leading-relaxed text-muted">
                    {isca.detalhe}
                  </span>
                  <span className="relative mt-auto pt-6 text-xs font-medium text-accent">
                    Abrir tela de erro · {data?.por_codigo[String(isca.codigo)] ?? 0} visitas
                  </span>
                </a>
              </li>
            ))}
          </ul>
        </nav>
      </section>

      <p className="mt-6 flex items-start gap-2 text-xs leading-relaxed text-muted">
        <WarningIcon className="shrink-0 text-warn" />
        {data?.total ?? 0} visitas ao todo. A contagem vive na memória do servidor e zera quando ele
        reinicia — ninguém está sendo registrado.
      </p>

      <div className="mt-10 border-t border-line pt-6">
        <h2 className="text-sm font-semibold">Por que isto existe</h2>
        {/* <p className="mt-2 max-w-lg text-sm leading-relaxed text-muted">
          As telas de erro do NAS foram desenhadas com capricho e, se tudo funcionar como deveria,
          ninguém nunca as veria. Estas portas garantem que o trabalho apareça. A de 404, aliás,
          é a mesma que responde quando você digita um endereço que não existe.
        </p> */}
        <Link to="/" className="mt-4 inline-block text-sm font-medium text-accent">
          Voltar ao acervo
        </Link>
      </div>
    </div>
  )
}
