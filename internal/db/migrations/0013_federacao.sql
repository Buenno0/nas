-- Federação Mac × instância cloud. Entrega at-least-once: todo evento
-- aplicado fica registrado, e o repetido é ignorado.
CREATE TABLE eventos_processados (
    event_id TEXT    PRIMARY KEY,
    em       INTEGER NOT NULL
);

CREATE INDEX idx_files_nuvem_key ON media_files (nuvem_key) WHERE nuvem_key != '';
