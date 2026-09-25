// Package db abre o SQLite e aplica as migrations embutidas.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // driver SQLite em Go puro, sem CGO

	"nas/internal/config"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// DB embrulha *sql.DB para os pacotes de domínio pendurarem queries nele.
type DB struct {
	*sql.DB
}

// Open abre o banco em ~/.nas/nas.db e deixa o esquema atualizado.
func Open() (*DB, error) {
	path, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	if _, err := config.EnsureDir(); err != nil {
		return nil, err
	}
	return OpenAt(path)
}

// OpenAt abre um banco em um caminho específico (usado nos testes).
func OpenAt(path string) (*DB, error) {
	dsn := "file:" + url.PathEscape(filepath.ToSlash(path)) + "?" + strings.Join([]string{
		"_pragma=journal_mode(WAL)",   // leitura durante o scan sem travar
		"_pragma=busy_timeout(5000)",  // espera em vez de estourar SQLITE_BUSY
		"_pragma=foreign_keys(1)",     // ON DELETE CASCADE precisa disso
		"_pragma=synchronous(NORMAL)", // seguro o suficiente com WAL
	}, "&")

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrindo banco: %w", err)
	}

	// Pool. Isto NÃO é ganho de latência: medido com 10 goroutines em paralelo,
	// a diferença contra o padrão do database/sql ficou dentro do ruído
	// (204µs contra 208µs por query). É um teto de recurso.
	//
	// O padrão abre conexões sem limite, e cada conexão SQLite carrega seu
	// próprio cache de páginas. Uma rajada — um celular abrindo a home enquanto
	// o scan indexa — poderia abrir dezenas e a memória subiria sem motivo, já
	// que SQLite serializa escrita e passar de um punhado de conexões não dá
	// vazão nenhuma. Guardar tantas ociosas quantas abertas evita fechar e
	// reabrir (cada abertura reexecuta os quatro _pragma do DSN).
	const conexoes = 8
	sqlDB.SetMaxOpenConns(conexoes)
	sqlDB.SetMaxIdleConns(conexoes)
	// Sem expiração: o banco é um arquivo local, uma conexão não "envelhece"
	// como a de um servidor remoto atrás de balanceador.
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("conectando ao banco: %w", err)
	}

	d := &DB{sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

// migrate aplica, em ordem, as migrations ainda não registradas. Cada arquivo
// roda dentro de uma transação junto com o registro da própria versão, então
// uma migration que falha no meio não deixa o banco em estado intermediário.
func (d *DB) migrate() error {
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name    TEXT    NOT NULL,
		applied_at INTEGER NOT NULL DEFAULT (unixepoch())
	)`); err != nil {
		return fmt.Errorf("criando schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := d.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("lendo migrations aplicadas: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: nome deve começar com número", name)
		}
		if applied[version] {
			continue
		}

		stmt, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}

		tx, err := d.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(stmt)); err != nil {
			tx.Rollback()
			return fmt.Errorf("aplicando %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, version, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit de %s: %w", name, err)
		}
	}
	return nil
}
