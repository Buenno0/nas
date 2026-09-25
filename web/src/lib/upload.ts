// Upload direto para o bucket: o navegador fatia o arquivo e manda cada parte
// ao S3 por uma URL assinada. Os bytes não passam pelo Mac.
//
// O kill switch pausa tudo: quem chama informa, por ativo(), se o modo ainda é
// híbrido. Partes em voo são abortadas; ao voltar, o bucket diz o que já tem e
// só o resto é enviado.
import { api, type ParteEnviada, type Upload } from './api'

const PARALELAS = 4
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

  const urls = new Map<number, string>()
  const urlDa = async (n: number) => {
    if (!urls.has(n)) {
      const lote = faltam.filter((m) => m >= n && !urls.has(m)).slice(0, URLS_POR_PEDIDO)
      const { urls: novas } = await api.urlsDoUpload(upload.id, lote)
      for (const [k, v] of Object.entries(novas)) urls.set(Number(k), v)
    }
    return urls.get(n)!
  }

  const enviaParte = (n: number, url: string) =>
    new Promise<string>((resolve, reject) => {
      const inicio = (n - 1) * upload.parte_tamanho
      const xhr = new XMLHttpRequest()
      xhr.open('PUT', url)
      xhr.upload.onprogress = (e) => {
        emVoo.set(n, e.loaded)
        avisa()
      }
      xhr.onload = () => {
        const etag = xhr.getResponseHeader('ETag')
        if (xhr.status >= 200 && xhr.status < 300 && etag) resolve(etag)
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
  const trabalhador = async () => {
    while (proxima < faltam.length) {
      if (controle.signal.aborted) throw new PausadoPeloModo()
      const n = faltam[proxima++]
      const etag = await enviaParte(n, await urlDa(n))
      emVoo.delete(n)
      feitas.set(n, etag)
      enviados += tamanhoDa(n)
      avisa()
    }
  }

  try {
    await Promise.all(Array.from({ length: Math.min(PARALELAS, faltam.length) }, trabalhador))
  } catch (e) {
    controle.abort()
    if (!ativo() || e instanceof PausadoPeloModo) {
      onProgresso({ enviados, total: upload.tamanho, pausado: true })
      throw new PausadoPeloModo()
    }
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
