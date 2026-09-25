-- Modo híbrido: cada arquivo passa a ter uma localização.
--
-- local    só no disco do Mac (tudo o que existia antes desta migration)
-- enviando upload em andamento; o arquivo ainda está no disco
-- ambos    no disco e no bucket
-- baixando fixando localmente um item da nuvem
-- nuvem    só no bucket; no modo local aparece como "na nuvem, indisponível"
--
-- O scanner nunca apaga linhas nuvem/baixando: o arquivo não sumiu, ele nunca
-- esteve no disco.
ALTER TABLE media_files ADD COLUMN localizacao TEXT NOT NULL DEFAULT 'local'
    CHECK (localizacao IN ('local', 'enviando', 'ambos', 'baixando', 'nuvem'));
ALTER TABLE media_files ADD COLUMN nuvem_key TEXT NOT NULL DEFAULT '';
ALTER TABLE media_files ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';

-- Estado do multipart. Sobrevive a reinícios e ao kill switch: ao voltar para
-- o híbrido, o upload continua de onde parou em vez de recomeçar.
CREATE TABLE uploads (
    id           INTEGER PRIMARY KEY,
    library_id   INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    user_id      INTEGER REFERENCES users (id) ON DELETE SET NULL,
    nuvem_key    TEXT    NOT NULL UNIQUE,
    upload_id    TEXT    NOT NULL,
    nome         TEXT    NOT NULL,
    tamanho      INTEGER NOT NULL,
    parte_tamanho INTEGER NOT NULL,
    content_type TEXT    NOT NULL DEFAULT '',
    -- Arquivo de origem no Mac (nas push); vazio quando veio do navegador.
    origem       TEXT    NOT NULL DEFAULT '',
    -- Arquivo já indexado que este upload espelha (push de item local).
    media_file_id INTEGER REFERENCES media_files (id) ON DELETE SET NULL,
    estado       TEXT    NOT NULL DEFAULT 'enviando'
        CHECK (estado IN ('enviando', 'concluido', 'abortado')),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE INDEX idx_uploads_estado ON uploads (estado);
