-- Papel de administrador. Quem manda no acervo (bibliotecas, scan, chave do
-- TMDB) é só o admin; os demais entram, assistem e mexem no que é deles.
ALTER TABLE users ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0;

-- O primeiro usuário do banco é quem instalou o servidor: vira admin.
UPDATE users SET is_admin = 1 WHERE id = (SELECT MIN(id) FROM users);
