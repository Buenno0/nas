package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrColecaoDuplicada distingue "esse nome já existe" de uma falha de banco:
// a interface precisa dizer isso à pessoa, não devolver erro genérico.
var ErrColecaoDuplicada = errors.New("já existe uma coleção com esse nome")

// Colecao é uma lista montada à mão. É por usuário: a maratona de um não é a
// do outro, do mesmo jeito que progresso e favoritos.
type Colecao struct {
	ID         int64  `json:"id"`
	Nome       string `json:"nome"`
	Itens      int    `json:"itens"`
	Poster     string `json:"poster,omitempty"`
	CriadaEm   int64  `json:"criada_em"`
	Atualizada int64  `json:"atualizada_em"`
}

// Colecoes lista as coleções da pessoa, a mais mexida primeiro.
func (d *DB) Colecoes(ctx context.Context, userID int64) ([]Colecao, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT c.id, c.name, c.created_at, c.updated_at,
		       (SELECT COUNT(*) FROM collection_items i WHERE i.collection_id = c.id),
		       IFNULL((SELECT t.poster FROM collection_items i
		                 JOIN titles t ON t.id = i.title_id
		                WHERE i.collection_id = c.id AND t.poster != ''
		                ORDER BY i.position, i.added_at LIMIT 1), '')
		  FROM collections c
		 WHERE c.user_id = ?
		 ORDER BY c.updated_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("listando coleções: %w", err)
	}
	defer rows.Close()

	cols := []Colecao{}
	for rows.Next() {
		var c Colecao
		if err := rows.Scan(&c.ID, &c.Nome, &c.CriadaEm, &c.Atualizada, &c.Itens, &c.Poster); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// CriarColecao cria a lista vazia.
func (d *DB) CriarColecao(ctx context.Context, userID int64, nome string) (Colecao, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return Colecao{}, errors.New("a coleção precisa de um nome")
	}
	agora := time.Now().Unix()

	var id int64
	err := d.QueryRowContext(ctx, `
		INSERT INTO collections (user_id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		RETURNING id`, userID, nome, agora, agora).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Colecao{}, ErrColecaoDuplicada
		}
		return Colecao{}, fmt.Errorf("criando coleção: %w", err)
	}
	return Colecao{ID: id, Nome: nome, CriadaEm: agora, Atualizada: agora}, nil
}

// RenomearColecao troca o nome, conferindo o dono na própria cláusula: sem o
// user_id no WHERE, o id da URL bastaria para mexer na lista de outra pessoa.
func (d *DB) RenomearColecao(ctx context.Context, userID, id int64, nome string) error {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return errors.New("a coleção precisa de um nome")
	}
	res, err := d.ExecContext(ctx,
		`UPDATE collections SET name = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		nome, time.Now().Unix(), id, userID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrColecaoDuplicada
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) ApagarColecao(ctx context.Context, userID, id int64) error {
	res, err := d.ExecContext(ctx, `DELETE FROM collections WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AdicionarNaColecao põe o título no fim da lista. Repetir não é erro: quem
// clicou duas vezes quis o mesmo resultado das duas.
func (d *DB) AdicionarNaColecao(ctx context.Context, userID, colecaoID, titleID int64) error {
	if err := d.confereDono(ctx, userID, colecaoID); err != nil {
		return err
	}
	agora := time.Now().Unix()
	_, err := d.ExecContext(ctx, `
		INSERT INTO collection_items (collection_id, title_id, position, added_at)
		VALUES (?, ?, (SELECT IFNULL(MAX(position), -1) + 1 FROM collection_items WHERE collection_id = ?), ?)
		ON CONFLICT (collection_id, title_id) DO NOTHING`,
		colecaoID, titleID, colecaoID, agora)
	if err != nil {
		return fmt.Errorf("adicionando à coleção: %w", err)
	}
	return d.marcaMexida(ctx, colecaoID)
}

func (d *DB) RemoverDaColecao(ctx context.Context, userID, colecaoID, titleID int64) error {
	if err := d.confereDono(ctx, userID, colecaoID); err != nil {
		return err
	}
	if _, err := d.ExecContext(ctx,
		`DELETE FROM collection_items WHERE collection_id = ? AND title_id = ?`,
		colecaoID, titleID); err != nil {
		return err
	}
	return d.marcaMexida(ctx, colecaoID)
}

// ReordenarColecao grava a ordem escolhida. A lista chega inteira, não em
// pares de troca: assim a ordem final é sempre a que a pessoa está vendo, mesmo
// se dois arrastes chegarem fora de ordem.
func (d *DB) ReordenarColecao(ctx context.Context, userID, colecaoID int64, titleIDs []int64) error {
	if err := d.confereDono(ctx, userID, colecaoID); err != nil {
		return err
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for pos, id := range titleIDs {
		if _, err := tx.ExecContext(ctx,
			`UPDATE collection_items SET position = ? WHERE collection_id = ? AND title_id = ?`,
			pos, colecaoID, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return d.marcaMexida(ctx, colecaoID)
}

// ItensDaColecao devolve os títulos na ordem escolhida.
func (d *DB) ItensDaColecao(ctx context.Context, userID, colecaoID int64) (Colecao, []TitleCard, error) {
	var c Colecao
	err := d.QueryRowContext(ctx,
		`SELECT id, name, created_at, updated_at FROM collections WHERE id = ? AND user_id = ?`,
		colecaoID, userID).Scan(&c.ID, &c.Nome, &c.CriadaEm, &c.Atualizada)
	if err != nil {
		return Colecao{}, nil, ErrNotFound
	}

	rows, err := d.QueryContext(ctx, titleCardSelect+`
	  JOIN collection_items i ON i.title_id = t.id AND i.collection_id = ?
	 ORDER BY i.position, i.added_at`, colecaoID)
	if err != nil {
		return Colecao{}, nil, fmt.Errorf("itens da coleção: %w", err)
	}
	defer rows.Close()

	cards, err := coletarCards(rows)
	if err != nil {
		return Colecao{}, nil, err
	}
	c.Itens = len(cards)
	return c, cards, nil
}

// ColecoesComTitulo diz em quais listas o título já está, para a interface
// marcar as caixas em vez de deixar a pessoa adivinhar.
func (d *DB) ColecoesComTitulo(ctx context.Context, userID, titleID int64) ([]int64, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT c.id FROM collections c
		  JOIN collection_items i ON i.collection_id = c.id
		 WHERE c.user_id = ? AND i.title_id = ?`, userID, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// confereDono é a autorização de verdade destas rotas: o id vem da URL, e sem
// esta checagem qualquer conta mexeria na lista de qualquer outra.
func (d *DB) confereDono(ctx context.Context, userID, colecaoID int64) error {
	var existe int
	err := d.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collections WHERE id = ? AND user_id = ?`, colecaoID, userID).Scan(&existe)
	if err != nil || existe == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) marcaMexida(ctx context.Context, colecaoID int64) error {
	_, err := d.ExecContext(ctx,
		`UPDATE collections SET updated_at = ? WHERE id = ?`, time.Now().Unix(), colecaoID)
	return err
}
