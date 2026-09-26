// Upload direto para o bucket: o navegador fatia o arquivo e manda cada parte
// ao S3 por uma URL assinada. Os bytes não passam pelo Mac.
//
// O kill switch pausa tudo: quem chama informa, por ativo(), se o modo ainda é
// híbrido. Partes em voo são abortadas; ao voltar, o bucket diz o que já tem e
// só o resto é enviado.
import { api, type ParteEnviada, type Upload } from './api'

const PARALELAS = 4
// Com o link de subida lento, 4 partes disputam a banda e alguma fica ~20 s
// sem enviar nada: o S3 encerra a conexão por inatividade ("falha de rede").
// Depois da primeira parte que cai, o envio segue com 2.
const PARALELAS_NO_APERTO = 2
const URLS_POR_PEDIDO = 20

export interface ProgressoEnvio {
  enviados: number
  total: number
  pausado: boolean
}

export class PausadoPeloModo extends Error {
  constructor() {
    super('modo local: envio pausado')
  }
}

export async function criarEnvio(arquivo: File, libraryId: number) {
  const { upload } = await api.criarUpload(
    libraryId,
    arquivo.name,
    arquivo.size,
    arquivo.type || 'application/octet-stream',
  )
  return upload
}

/**
 * Envia (ou retoma) as partes que faltam e conclui. Lança PausadoPeloModo se o
 * kill switch for acionado no meio; chamar de novo depois retoma.
 */
export async function enviar(
  arquivo: File,
  upload: Upload,
  ativo: () => boolean,
  onProgresso: (p: ProgressoEnvio) => void,
): Promise<number> {
  const total = Math.ceil(upload.tamanho / upload.parte_tamanho)
  const { enviadas } = await api.partesDoUpload(upload.id)
  const feitas = new Map<number, string>(enviadas.map((p) => [p.n, p.etag]))
  const tamanhoDa = (n: number) =>
    Math.min(upload.parte_tamanho, upload.tamanho - (n - 1) * upload.parte_tamanho)

  let enviados = [...feitas.keys()].reduce((soma, n) => soma + tamanhoDa(n), 0)
  if (feitas.size > 0) api.anotarEnvio(upload.id, { tipo: 'retomada' })
  const emVoo = new Map<number, number>()
  const avisa = () => {
    const parcial = [...emVoo.values()].reduce((a, b) => a + b, 0)
    onProgresso({ enviados: enviados + parcial, total: upload.tamanho, pausado: false })
  }
  avisa()

  const faltam: number[] = []
  for (let n = 1; n <= total; n++) if (!feitas.has(n)) faltam.push(n)

  const controle = new AbortController()
  const vigia = window.setInterval(() => {
    if (!ativo()) controle.abort()
  }, 200)

  // Um pedido de URLs por vez: as partes paralelas esperam o mesmo lote em
  // vez de cada uma pedir 20 URLs quase iguais ao mesmo tempo.
  const urls = new Map<number, string>()
  let pedindo: Promise<void> | null = null
  const urlDa = async (n: number): Promise<string> => {
    while (!urls.has(n)) {
      if (!pedindo) {
        const lote = faltam.filter((m) => m >= n && !urls.has(m)).slice(0, URLS_POR_PEDIDO)
        pedindo = api
          .urlsDoUpload(upload.id, lote)
          .then(({ urls: novas }) => {
            for (const [k, v] of Object.entries(novas)) urls.set(Number(k), v)
          })
          .finally(() => {
            pedindo = null
          })
      }
      await pedindo
    }
    return urls.get(n)!
  }

  // Uma parte que cai no meio (Wi-Fi que oscila, celular trocando de antena)
  // tenta de novo antes de derrubar o arquivo inteiro. O kill switch não:
  // ele pausa na hora. A URL assinada vale 1 h, então é reaproveitada.
  const esperas = [2000, 5000, 10000]
  let limite = PARALELAS
  const enviaComTentativas = async (n: number) => {
    for (let tentativa = 0; ; tentativa++) {
      try {
        return await enviaParte(n, await urlDa(n))
      } catch (e) {
        if (e instanceof PausadoPeloModo || controle.signal.aborted || tentativa >= esperas.length) throw e
        emVoo.delete(n)
        avisa()
        limite = Math.min(limite, PARALELAS_NO_APERTO)
        api.anotarEnvio(upload.id, { tipo: 'erro', erro: `parte ${n}: ${(e as Error).message}; tentando de novo (${tentativa + 1}/${esperas.length})` })
        await new Promise((r) => window.setTimeout(r, esperas[tentativa]))
      }
    }
  }

  const enviaParte = (n: number, url: string) =>
    new Promise<string>((resolve, reject) => {
      const inicio = (n - 1) * upload.parte_tamanho
      const t0 = performance.now()
      const xhr = new XMLHttpRequest()
      xhr.open('PUT', url)
      xhr.upload.onprogress = (e) => {
        emVoo.set(n, e.loaded)
        avisa()
      }
      xhr.onload = () => {
        const etag = xhr.getResponseHeader('ETag')
        if (xhr.status >= 200 && xhr.status < 300 && etag) {
          api.anotarEnvio(upload.id, { tipo: 'parte', n, tamanho: tamanhoDa(n), ms: Math.round(performance.now() - t0), etag })
          resolve(etag)
        }
        else if (!etag && xhr.status < 300)
          reject(new Error('o bucket não expôs o ETag: confira o CORS (ExposeHeaders: ETag)'))
        else reject(new Error(`parte ${n}: ${xhr.status}`))
      }
      xhr.onerror = () => reject(new Error(`parte ${n}: falha de rede`))
      xhr.onabort = () => reject(new PausadoPeloModo())
      controle.signal.addEventListener('abort', () => xhr.abort(), { once: true })
      xhr.send(arquivo.slice(inicio, inicio + tamanhoDa(n)))
    })

  let proxima = 0
  const trabalhador = async (id: number) => {
    // Os trabalhadores acima do limite encerram depois da parte em que estão.
    while (proxima < faltam.length && id < limite) {
      if (controle.signal.aborted) throw new PausadoPeloModo()
      const n = faltam[proxima++]
      const etag = await enviaComTentativas(n)
      emVoo.delete(n)
      feitas.set(n, etag)
      enviados += tamanhoDa(n)
      avisa()
    }
  }

  try {
    await Promise.all(Array.from({ length: Math.min(PARALELAS, faltam.length) }, (_, id) => trabalhador(id)))
  } catch (e) {
    controle.abort()
    if (!ativo() || e instanceof PausadoPeloModo) {
      onProgresso({ enviados, total: upload.tamanho, pausado: true })
      api.anotarEnvio(upload.id, { tipo: 'pausa' })
      throw new PausadoPeloModo()
    }
    api.anotarEnvio(upload.id, { tipo: 'erro', erro: (e as Error).message })
    throw e
  } finally {
    window.clearInterval(vigia)
  }

  const partes: ParteEnviada[] = [...feitas.entries()]
    .map(([n, etag]) => ({ n, etag }))
    .sort((a, b) => a.n - b.n)
  const { file_id } = await api.concluirUpload(upload.id, partes)
  return file_id
}
