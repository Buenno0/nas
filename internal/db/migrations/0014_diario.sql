-- Diário técnico: o que a nuvem fez e quando (envios, partes, jobs, cortes do
-- kill switch). Só leitura para a tela técnica; podado aos 30 dias.
CREATE TABLE diario (
    id        INTEGER PRIMARY KEY,
    em        INTEGER NOT NULL, -- milissegundos
    tipo      TEXT    NOT NULL,
    upload_id INTEGER,
    file_id   INTEGER,
    nome      TEXT    NOT NULL DEFAULT '',
    dados     TEXT    NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_diario_upload ON diario (upload_id) WHERE upload_id IS NOT NULL;
