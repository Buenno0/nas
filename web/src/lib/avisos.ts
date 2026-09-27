// Avisos no celular (Web Push). No iPhone só funcionam com o site adicionado
// à Tela de Início (iOS 16.4+); em outros navegadores, direto.
import { useCallback, useEffect, useState } from 'react'

export type TipoDeAviso = 'envio' | 'preparo' | 'novidade'

export const TIPOS_DE_AVISO: { tipo: TipoDeAviso; rotulo: string; detalhe: string }[] = [
  { tipo: 'envio', rotulo: 'Envios concluídos', detalhe: 'quando um arquivo termina de subir para a nuvem' },
  { tipo: 'preparo', rotulo: 'Pronto para assistir', detalhe: 'quando o worker prepara um filme para todos os aparelhos' },
  { tipo: 'novidade', rotulo: 'Novidades no acervo', detalhe: 'quando entra filme, série ou álbum novo' },
]

type Suporte = 'sim' | 'instalar' | 'nao'

/** 'instalar' = iPhone/iPad fora da Tela de Início: precisa adicionar antes. */
function suporte(): Suporte {
  const ios = /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
  const instalado = window.matchMedia?.('(display-mode: standalone)').matches || (navigator as { standalone?: boolean }).standalone === true
  if (!('serviceWorker' in navigator)) return ios && !instalado ? 'instalar' : 'nao'
  if (!('PushManager' in window) || !('Notification' in window)) return ios && !instalado ? 'instalar' : 'nao'
  return 'sim'
}

function chaveEmBytes(base64: string): Uint8Array {
  const pad = '='.repeat((4 - (base64.length % 4)) % 4)
  const bruto = atob((base64 + pad).replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(bruto, (c) => c.charCodeAt(0))
}

async function json<T>(caminho: string, init?: RequestInit): Promise<T> {
  const res = await fetch(caminho, {
    credentials: 'same-origin',
    headers: init?.body ? { 'Content-Type': 'application/json' } : undefined,
    ...init,
  })
  if (!res.ok) throw new Error((await res.json().catch(() => ({})))?.error ?? `HTTP ${res.status}`)
  return res.status === 204 || res.status === 202 ? (undefined as T) : res.json()
}

async function registro() {
  return navigator.serviceWorker.register('/sw.js', { scope: '/' })
}

export function useAvisos() {
  const [estado] = useState<Suporte>(suporte)
  const [inscrito, setInscrito] = useState(false)
  const [tipos, setTipos] = useState<TipoDeAviso[]>(['envio', 'preparo', 'novidade'])
  const [ocupado, setOcupado] = useState(false)
  const [erro, setErro] = useState('')
  const [permissao, setPermissao] = useState<NotificationPermission>(() =>
    'Notification' in window ? Notification.permission : 'default',
  )

  // Já inscrito? O servidor responde pelo endpoint deste aparelho.
  useEffect(() => {
    if (estado !== 'sim') return
    void (async () => {
      const sub = await (await registro()).pushManager.getSubscription()
      if (!sub) return
      const r = await json<{ inscrito: boolean; tipos: TipoDeAviso[] }>(`/api/push?endpoint=${encodeURIComponent(sub.endpoint)}`).catch(() => null)
      if (r?.inscrito) {
        setInscrito(true)
        if (r.tipos.length) setTipos(r.tipos)
      }
    })()
  }, [estado])

  const salvar = useCallback(async (quais: TipoDeAviso[]) => {
    setOcupado(true)
    setErro('')
    try {
      const reg = await registro()
      const p = await Notification.requestPermission()
      setPermissao(p)
      if (p !== 'granted') throw new Error('As notificações foram bloqueadas nos ajustes do navegador.')
      let sub = await reg.pushManager.getSubscription()
      if (!sub) {
        const { chave } = await json<{ chave: string }>('/api/push/chave')
        sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: chaveEmBytes(chave) as BufferSource })
      }
      const r = await json<{ tipos: TipoDeAviso[] }>('/api/push', { method: 'POST', body: JSON.stringify({ ...sub.toJSON(), tipos: quais }) })
      setTipos(r.tipos)
      setInscrito(true)
    } catch (e) {
      setErro((e as Error).message)
    } finally {
      setOcupado(false)
    }
  }, [])

  const desligar = useCallback(async () => {
    setOcupado(true)
    setErro('')
    try {
      const sub = await (await registro()).pushManager.getSubscription()
      if (sub) {
        await json('/api/push', { method: 'DELETE', body: JSON.stringify({ endpoint: sub.endpoint }) })
        await sub.unsubscribe()
      }
      setInscrito(false)
    } catch (e) {
      setErro((e as Error).message)
    } finally {
      setOcupado(false)
    }
  }, [])

  const testar = useCallback(() => json('/api/push/teste', { method: 'POST' }).catch((e) => setErro((e as Error).message)), [])

  return { suporte: estado, inscrito, tipos, ocupado, erro, permissao, salvar, desligar, testar }
}
