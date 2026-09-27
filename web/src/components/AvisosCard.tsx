import { useState } from 'react'
import { TIPOS_DE_AVISO, useAvisos, type TipoDeAviso } from '../lib/avisos'
import { NuvemIcon } from './icons'

/** "Avisos neste aparelho": liga o Web Push e escolhe o que avisar. */
export function AvisosCard({ admin, naNuvem }: { admin: boolean; naNuvem: boolean }) {
  const a = useAvisos()
  // Envios e preparo são de quem mantém o NAS; novidades, de todo mundo.
  const disponiveis = TIPOS_DE_AVISO.filter((t) => admin || t.tipo === 'novidade')
  const [escolha, setEscolha] = useState<TipoDeAviso[] | null>(null)
  const marcados = escolha ?? a.tipos.filter((t) => disponiveis.some((d) => d.tipo === t))
  const alterna = (t: TipoDeAviso) =>
    setEscolha(marcados.includes(t) ? marcados.filter((x) => x !== t) : [...marcados, t])
  const mudou = escolha !== null && a.inscrito

  return (
    <section className="rounded-xl border border-line bg-surface p-5">
      <h2 className="text-sm font-semibold">Avisos neste aparelho</h2>
      <p className="mt-1 text-xs leading-relaxed text-muted">
        Notificações no celular ou no computador, pelo próprio navegador, sem app e sem custo. Valem para este
        endereço ({naNuvem ? 'a nuvem' : 'o Mac'}): se você usa os dois, ligue em cada um.
      </p>

      {a.suporte === 'instalar' ? (
        <div className="mt-4 rounded-lg bg-elev/60 px-4 py-3 text-sm">
          <p className="font-medium">No iPhone, adicione o Ozymandias à Tela de Início primeiro.</p>
          <p className="mt-1 text-xs text-muted">
            No Safari, toque em Compartilhar e depois em “Adicionar à Tela de Início”. Abra pelo ícone novo e volte
            aqui: os avisos aparecem como de um app.
          </p>
        </div>
      ) : a.suporte === 'nao' ? (
        <p className="mt-4 text-sm text-muted">Este navegador não recebe avisos.</p>
      ) : (
        <div className="mt-4 space-y-4">
          <ul className="space-y-2">
            {disponiveis.map((t) => (
              <li key={t.tipo}>
                <label className="flex cursor-pointer items-start gap-3 text-sm">
                  <input
                    type="checkbox"
                    checked={marcados.includes(t.tipo)}
                    onChange={() => alterna(t.tipo)}
                    className="mt-0.5 h-4 w-4 accent-[var(--accent)]"
                  />
                  <span>
                    <span className="font-medium">{t.rotulo}</span>
                    <span className="block text-xs text-muted">{t.detalhe}</span>
                  </span>
                </label>
              </li>
            ))}
          </ul>

          <div className="flex flex-wrap items-center gap-2">
            {!a.inscrito || mudou ? (
              <button
                type="button"
                disabled={a.ocupado || marcados.length === 0}
                onClick={() => void a.salvar(marcados).then(() => setEscolha(null))}
                className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-accent px-4 text-sm font-bold text-accent-ink transition hover:opacity-90 disabled:opacity-50"
              >
                <NuvemIcon estado={a.ocupado ? 'conectando' : 'hibrido'} />
                {a.inscrito ? 'Salvar escolha' : 'Ligar avisos'}
              </button>
            ) : (
              <span className="inline-flex items-center gap-2 rounded-lg bg-ok/10 px-3 py-2 text-sm font-semibold text-ok">
                <NuvemIcon estado="concluido" /> Avisos ligados
              </span>
            )}
            {a.inscrito && (
              <>
                <button type="button" onClick={() => void a.testar()} className="min-h-10 rounded-lg border border-line px-3 text-sm transition hover:bg-elev">
                  Mandar um teste
                </button>
                <button type="button" disabled={a.ocupado} onClick={() => void a.desligar()} className="min-h-10 rounded-lg px-3 text-sm text-muted transition hover:text-danger">
                  Desligar
                </button>
              </>
            )}
          </div>
          {a.permissao === 'denied' && (
            <p className="text-xs text-warn">As notificações estão bloqueadas nos ajustes do navegador para este site.</p>
          )}
          {a.erro && <p className="text-xs text-danger">{a.erro}</p>}
        </div>
      )}
    </section>
  )
}
