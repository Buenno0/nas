import { useRef, type ReactNode } from 'react'
import { ChevronLeft, ChevronRight } from './icons'

interface RowProps {
  title: string
  action?: ReactNode
  children: ReactNode
}

/** Prateleira horizontal com snap e setas no desktop. */
export function Row({ title, action, children }: RowProps) {
  const trackRef = useRef<HTMLDivElement>(null)

  const scrollBy = (direction: 1 | -1) => {
    const el = trackRef.current
    if (!el) return
    el.scrollBy({ left: direction * Math.round(el.clientWidth * 0.85), behavior: 'smooth' })
  }

  return (
    <section className="group/row relative">
      <div className="mb-3 flex items-end justify-between gap-4 px-4 sm:px-6">
        <h2 className="text-base font-semibold tracking-tight sm:text-lg">{title}</h2>
        {action}
      </div>

      <div className="relative">
        <button
          type="button"
          onClick={() => scrollBy(-1)}
          aria-label="Rolar para a esquerda"
          className="absolute top-0 bottom-14 left-0 z-10 hidden w-10 items-center justify-center bg-gradient-to-r from-bg to-transparent text-ink opacity-0 transition group-hover/row:opacity-100 focus-visible:opacity-100 lg:flex"
        >
          <ChevronLeft />
        </button>

        <div
          ref={trackRef}
          className="no-scrollbar flex snap-x snap-mandatory gap-3 overflow-x-auto scroll-smooth px-4 pb-2 sm:gap-4 sm:px-6"
        >
          {children}
        </div>

        <button
          type="button"
          onClick={() => scrollBy(1)}
          aria-label="Rolar para a direita"
          className="absolute top-0 right-0 bottom-14 z-10 hidden w-10 items-center justify-center bg-gradient-to-l from-bg to-transparent text-ink opacity-0 transition group-hover/row:opacity-100 focus-visible:opacity-100 lg:flex"
        >
          <ChevronRight />
        </button>
      </div>
    </section>
  )
}

/** Largura padrão de um item dentro da prateleira. */
export function RowItem({ children }: { children: ReactNode }) {
  return <div className="w-32 shrink-0 snap-start sm:w-36 md:w-40">{children}</div>
}
