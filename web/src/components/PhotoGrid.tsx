import { useCallback, useEffect, useState } from 'react'
import type { FileInfo } from '../lib/api'
import { streamUrl } from '../lib/api'
import { ChevronLeft, ChevronRight, DownloadIcon } from './icons'

const thumbUrl = (id: number, width: 320 | 800 | 1600 = 320) => `/img/file/${id}?w=${width}`

// HEIC não abre no Chrome: para esses, mostramos a versão convertida pelo
// servidor em vez do arquivo original.
const needsConversion = (ext: string) => ['.heic', '.heif', '.avif'].includes(ext.toLowerCase())

const fullUrl = (file: FileInfo) =>
  needsConversion(file.ext) ? thumbUrl(file.id, 1600) : streamUrl(file.id)

/**
 * Agrupa por mês, preservando a ordem que veio do servidor (mais recente
 * primeiro). O índice global de cada foto é guardado junto: o lightbox navega
 * pela galeria inteira, atravessando os meses, e não por um mês só.
 */
function porMes(photos: FileInfo[]) {
  const meses: { chave: string; rotulo: string; itens: { photo: FileInfo; i: number }[] }[] = []

  photos.forEach((photo, i) => {
    if (!photo.quando) {
      const solto = meses.find((m) => m.chave === 'sem-data')
      if (solto) solto.itens.push({ photo, i })
      else meses.push({ chave: 'sem-data', rotulo: 'Sem data', itens: [{ photo, i }] })
      return
    }
    const d = new Date(photo.quando * 1000)
    const chave = `${d.getFullYear()}-${d.getMonth()}`
    const atual = meses[meses.length - 1]
    if (atual && atual.chave === chave) {
      atual.itens.push({ photo, i })
      return
    }
    meses.push({
      chave,
      rotulo: d.toLocaleDateString('pt-BR', { month: 'long', year: 'numeric' }),
      itens: [{ photo, i }],
    })
  })

  return meses
}

const grade = 'grid grid-cols-3 gap-1.5 sm:grid-cols-4 sm:gap-2 md:grid-cols-6 xl:grid-cols-8'

export function PhotoGrid({ photos }: { photos: FileInfo[] }) {
  const [open, setOpen] = useState<number | null>(null)

  if (photos.length === 0) return null

  const meses = porMes(photos)
  // Uma única faixa de tempo não é linha do tempo: o cabeçalho seria só ruído.
  const agrupar = meses.length > 1

  return (
    <>
      {agrupar ? (
        <div className="space-y-8">
          {meses.map((mes) => (
            <section key={mes.chave}>
              <h3 className="mb-2 text-sm font-semibold first-letter:uppercase">{mes.rotulo}</h3>
              <ul className={grade}>
                {mes.itens.map(({ photo, i }) => (
                  <Miniatura key={photo.id} photo={photo} onOpen={() => setOpen(i)} />
                ))}
              </ul>
            </section>
          ))}
        </div>
      ) : (
        <ul className={grade}>
          {photos.map((photo, i) => (
            <Miniatura key={photo.id} photo={photo} onOpen={() => setOpen(i)} />
          ))}
        </ul>
      )}

      {open !== null && (
        <Lightbox
          photos={photos}
          index={open}
          onIndex={setOpen}
          onClose={() => setOpen(null)}
        />
      )}
    </>
  )
}

function Miniatura({ photo, onOpen }: { photo: FileInfo; onOpen: () => void }) {
  return (
    <li className="contain-content">
      <button
        type="button"
        onClick={onOpen}
        className="group block aspect-square w-full overflow-hidden rounded-lg bg-elev"
        aria-label={`Abrir ${photo.name}`}
      >
        <img
          src={thumbUrl(photo.id)}
          alt=""
          loading="lazy"
          decoding="async"
          className="h-full w-full object-cover transition duration-200 group-hover:scale-105"
        />
      </button>
    </li>
  )
}

function Lightbox({
  photos,
  index,
  onIndex,
  onClose,
}: {
  photos: FileInfo[]
  index: number
  onIndex: (i: number) => void
  onClose: () => void
}) {
  const photo = photos[index]

  const go = useCallback(
    (delta: number) => {
      const next = index + delta
      if (next >= 0 && next < photos.length) onIndex(next)
    },
    [index, onIndex, photos.length],
  )

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      if (e.key === 'ArrowRight') go(1)
      if (e.key === 'ArrowLeft') go(-1)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [go, onClose])

  if (!photo) return null

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={photo.name}
      className="fixed inset-0 z-50 flex flex-col bg-black/95"
      onClick={onClose}
    >
      <div className="flex items-center gap-3 p-4 text-white" onClick={(e) => e.stopPropagation()}>
        <button
          type="button"
          onClick={onClose}
          aria-label="Fechar"
          className="grid h-10 w-10 place-items-center rounded-full bg-white/10 transition hover:bg-white/20"
        >
          <ChevronLeft />
        </button>
        <div className="min-w-0">
          <p className="line-clamp-1 text-sm font-medium">{photo.name}</p>
          <p className="text-xs text-white/60">
            {index + 1} de {photos.length}
          </p>
        </div>
        <a
          href={`${streamUrl(photo.id)}?download=1`}
          onClick={(e) => e.stopPropagation()}
          aria-label="Baixar foto"
          className="ml-auto grid h-10 w-10 place-items-center rounded-full bg-white/10 transition hover:bg-white/20"
        >
          <DownloadIcon />
        </a>
      </div>

      <div className="relative flex min-h-0 flex-1 items-center justify-center p-2">
        <img
          src={fullUrl(photo)}
          alt={photo.name}
          onClick={(e) => e.stopPropagation()}
          className="max-h-full max-w-full object-contain"
        />

        {index > 0 && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              go(-1)
            }}
            aria-label="Foto anterior"
            className="absolute left-2 grid h-11 w-11 place-items-center rounded-full bg-black/50 text-white transition hover:bg-black/70"
          >
            <ChevronLeft />
          </button>
        )}
        {index + 1 < photos.length && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              go(1)
            }}
            aria-label="Próxima foto"
            className="absolute right-2 grid h-11 w-11 place-items-center rounded-full bg-black/50 text-white transition hover:bg-black/70"
          >
            <ChevronRight />
          </button>
        )}
      </div>
    </div>
  )
}
