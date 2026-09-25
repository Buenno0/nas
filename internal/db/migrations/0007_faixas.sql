-- Faixas de áudio e legendas de cada arquivo.
--
-- Até aqui o NAS guardava UM codec de áudio por arquivo (o primeiro) e nenhuma
-- legenda. Um MKV "Dual Áudio" perdia a faixa original; um MKV legendado perdia
-- a legenda inteira, porque toda receita de transcodificação passava
-- `-map 0:a:0 -sn`. O acervo tinha as duas coisas e o servidor descartava as
-- duas — o próprio nameparse já reconhece "dual", "dublado" e "legendado" nos
-- nomes dos arquivos.
CREATE TABLE media_streams (
    media_file_id INTEGER NOT NULL REFERENCES media_files (id) ON DELETE CASCADE,

    -- Índice do stream DENTRO do arquivo, como o ffmpeg o endereça em -map 0:N.
    -- Legendas em arquivo separado (filme.mkv + filme.pt.srt) usam índice
    -- negativo: não existem dentro do container, mas precisam de identidade
    -- estável para virar URL.
    idx           INTEGER NOT NULL,

    kind          TEXT    NOT NULL CHECK (kind IN ('audio', 'subtitle')),
    codec         TEXT    NOT NULL DEFAULT '',
    lang          TEXT    NOT NULL DEFAULT '',
    title         TEXT    NOT NULL DEFAULT '',
    channels      INTEGER NOT NULL DEFAULT 0,
    is_default    INTEGER NOT NULL DEFAULT 0,
    forced        INTEGER NOT NULL DEFAULT 0,

    -- Caminho absoluto, só para legenda em arquivo ao lado.
    ext_path      TEXT    NOT NULL DEFAULT '',

    PRIMARY KEY (media_file_id, idx)
);

-- Arquivos já indexados não têm faixas registradas. Zerar probed_at obriga o
-- próximo scan a reexaminá-los, em vez de exigir `nas scan --force` do usuário.
UPDATE media_files SET probed_at = NULL WHERE media_type = 'video';

ANALYZE;
