// Chromecast pelo navegador (Google Cast Web Sender).
//
// O SDK só existe no Chrome e só é baixado quando o player abre. Quem toca é o
// receptor padrão da Google, que abre a URL sozinho, sem o cookie da sessão:
// por isso a mídia vai com o token de mídia na query, como no AirPlay do iOS.
import { useEffect, useState } from 'react'

// Receptor padrão da Google: toca MP4/WebM/HLS sem app próprio registrado.
const RECEPTOR_PADRAO = 'CC1AD845'
const SDK = 'https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1'

/* eslint-disable @typescript-eslint/no-explicit-any */
type CastWindow = Window & {
  cast?: any
  chrome?: any
  __onGCastApiAvailable?: (ok: boolean) => void
}
const w = window as CastWindow

let carregando: Promise<boolean> | undefined

function carregarSdk(): Promise<boolean> {
  if (carregando) return carregando
  // Sem window.chrome não há Cast: Safari e Firefox nem baixam o script.
  if (!w.chrome) return (carregando = Promise.resolve(false))
  carregando = new Promise<boolean>((resolve) => {
    w.__onGCastApiAvailable = (ok) => {
      if (ok) {
        w.cast.framework.CastContext.getInstance().setOptions({
          receiverApplicationId: RECEPTOR_PADRAO,
          autoJoinPolicy: w.chrome.cast.AutoJoinPolicy.ORIGIN_SCOPED,
        })
      }
      resolve(ok)
    }
    const s = document.createElement('script')
    s.src = SDK
    s.async = true
    s.onerror = () => resolve(false)
    document.head.appendChild(s)
  })
  return carregando
}

export interface MidiaParaCast {
  url: string
  titulo: string
  capa?: string
  inicio: number
  legenda?: { url: string; lang: string; rotulo: string }
}

export interface EstadoCast {
  /** Há SDK e ao menos um aparelho na rede. */
  disponivel: boolean
  /** Nome da TV quando há uma sessão aberta. */
  tv?: string
}

export function useCast(): EstadoCast {
  const [estado, setEstado] = useState<EstadoCast>({ disponivel: false })

  useEffect(() => {
    let vivo = true
    let desligar = () => {}
    void carregarSdk().then((ok) => {
      if (!ok || !vivo) return
      const fw = w.cast.framework
      const ctx = fw.CastContext.getInstance()
      const atualizar = () => {
        const sessao = ctx.getCurrentSession()
        setEstado({
          disponivel: ctx.getCastState() !== fw.CastState.NO_DEVICES_AVAILABLE,
          tv: sessao?.getCastDevice()?.friendlyName,
        })
      }
      atualizar()
      ctx.addEventListener(fw.CastContextEventType.CAST_STATE_CHANGED, atualizar)
      desligar = () => ctx.removeEventListener(fw.CastContextEventType.CAST_STATE_CHANGED, atualizar)
    })
    return () => {
      vivo = false
      desligar()
    }
  }, [])

  return estado
}

/** Abre o seletor de TVs (se preciso) e manda a mídia para lá. */
export async function castar(m: MidiaParaCast): Promise<void> {
  if (!(await carregarSdk())) throw new Error('Chromecast só funciona no Chrome')
  const ctx = w.cast.framework.CastContext.getInstance()
  if (!ctx.getCurrentSession()) await ctx.requestSession()
  const sessao = ctx.getCurrentSession()
  if (!sessao) return

  const c = w.chrome.cast.media
  const info = new c.MediaInfo(m.url, 'video/mp4')
  info.metadata = new c.GenericMediaMetadata()
  info.metadata.title = m.titulo
  if (m.capa) info.metadata.images = [new w.chrome.cast.Image(m.capa)]

  const pedido = new c.LoadRequest(info)
  pedido.currentTime = m.inicio
  pedido.autoplay = true

  if (m.legenda) {
    const faixa = new c.Track(1, c.TrackType.TEXT)
    faixa.trackContentId = m.legenda.url
    faixa.trackContentType = 'text/vtt'
    faixa.subtype = c.TextTrackType.SUBTITLES
    faixa.name = m.legenda.rotulo
    faixa.language = m.legenda.lang
    info.tracks = [faixa]
    pedido.activeTrackIds = [1]
  }

  await sessao.loadMedia(pedido)
}

export function pararCast() {
  w.cast?.framework?.CastContext.getInstance().endCurrentSession(true)
}
