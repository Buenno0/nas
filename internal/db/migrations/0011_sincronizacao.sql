-- Biblioteca espelhada: no modo híbrido, todo arquivo local dela ganha uma
-- cópia no bucket (fica "ambos").
ALTER TABLE libraries ADD COLUMN espelhada INTEGER NOT NULL DEFAULT 0;

-- Outbox: tudo o que muda no Mac vira evento aqui, também no modo local (é
-- barato e não sai da máquina). Ao entrar no híbrido, o que acumulou é
-- drenado para o journal no bucket. Alternar de modo não perde nada.
CREATE TABLE outbox (
    id      INTEGER PRIMARY KEY,
    tipo    TEXT    NOT NULL,
    payload TEXT    NOT NULL,
    criado  INTEGER NOT NULL
);

-- Pequenos valores da sincronização: último journal enviado, última
-- reconciliação do catálogo com o bucket.
CREATE TABLE nuvem_estado (
    chave TEXT PRIMARY KEY,
    valor TEXT NOT NULL
);
