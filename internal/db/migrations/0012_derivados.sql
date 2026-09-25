-- Derivados que o worker da nuvem gerou para itens do bucket: MP4 compatível,
-- miniaturas, WebVTT. Vêm do evento job.concluido; nunca do banco da nuvem.
CREATE TABLE derivados (
    media_file_id INTEGER NOT NULL REFERENCES media_files (id) ON DELETE CASCADE,
    tipo          TEXT    NOT NULL,
    indice        INTEGER NOT NULL DEFAULT 0,
    nuvem_key     TEXT    NOT NULL,
    receita       TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (media_file_id, tipo, indice)
);

-- Pedido de processamento feito pelo Mac, para o player saber se espera.
CREATE TABLE processamento (
    media_file_id INTEGER PRIMARY KEY REFERENCES media_files (id) ON DELETE CASCADE,
    estado        TEXT    NOT NULL CHECK (estado IN ('pedido', 'concluido', 'falhou')),
    erro          TEXT    NOT NULL DEFAULT '',
    atualizado    INTEGER NOT NULL
);
