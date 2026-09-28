-- Tempo assistido junto, por pessoa e companhia, por dia: a retrospectiva
-- mostra com quem mais se assistiu. `com` é o nome de usuário (a companhia
-- pode ser de outra instalação no futuro, então não é chave estrangeira).
CREATE TABLE juntos_historico (
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    com      TEXT    NOT NULL,
    dia      TEXT    NOT NULL, -- AAAA-MM-DD, horário local do nó
    segundos REAL    NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, com, dia)
);
