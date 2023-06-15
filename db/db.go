package db

import (
	"bytes"
	"context"
	"feedrewind/config"
	"feedrewind/db/migrations"
	"feedrewind/db/pgw"
	"fmt"
	"go/token"
	"hash/crc32"
	"os"
	"os/exec"
	"text/template"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var DbCmd *cobra.Command

func init() {
	DbCmd = &cobra.Command{
		Use: "db",
	}

	dumpCmd := &cobra.Command{
		Use: "dump",
		Run: func(_ *cobra.Command, _ []string) {
			dumpStructure()
		},
	}

	generateMigraionCmd := &cobra.Command{
		Use:  "generate-migration",
		Args: cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			generateMigration(args[0])
		},
	}

	migrateCmd := &cobra.Command{
		Use: "migrate",
		Run: func(_ *cobra.Command, _ []string) {
			migrate()
		},
	}

	rollbackCmd := &cobra.Command{
		Use: "rollback",
		Run: func(_ *cobra.Command, _ []string) {
			rollback()
		},
	}

	DbCmd.AddCommand(dumpCmd)
	DbCmd.AddCommand(generateMigraionCmd)
	DbCmd.AddCommand(migrateCmd)
	DbCmd.AddCommand(rollbackCmd)
}

var Conn *pgw.Pool

func init() {
	var err error
	Conn, err = pgw.NewPool(context.Background(), config.Cfg.DB.DSN())
	if err != nil {
		panic(err)
	}
}

func dumpStructure() {
	filename := "db/structure.sql"

	pgDump, err := exec.LookPath("pg_dump")
	if err != nil {
		panic(err)
	}
	pgDumpCmd := exec.Command(
		pgDump, "--schema-only", "--no-privileges", "--no-owner", "--file", filename, config.Cfg.DB.DBName,
	)
	pgDumpCmd.Stdout = os.Stdout
	pgDumpCmd.Stderr = os.Stderr
	err = pgDumpCmd.Run()
	if err != nil {
		panic(err)
	}

	rows, err := Conn.Query(
		context.Background(), "select version from schema_migrations order by version asc",
	)
	if err != nil {
		panic(err)
	}
	var versions []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			panic(err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()

	fmt.Fprintln(file)
	fmt.Fprintln(file, `INSERT INTO "schema_migrations" (version) VALUES`)
	for i, version := range versions {
		sep := ','
		if i == len(versions)-1 {
			sep = ';'
		}
		fmt.Fprintf(file, "('%s')%c\n", version, sep)
	}
}

func generateMigration(name string) {
	if !token.IsIdentifier(name) {
		panic(fmt.Errorf("migration name is not a valid identifier: %s", name))
	}
	if !token.IsExported(name) {
		panic(fmt.Errorf("migration name is not an exported identifier: %s", name))
	}

	version := time.Now().Format("20060102030405")
	templateParams := struct {
		Version    string
		StructName string
	}{
		Version:    version,
		StructName: name,
	}

	templateText := `package migrations

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type {{.StructName}} struct {}

func init() {
	registerMigration(&{{.StructName}}{})
}

func (m *{{.StructName}}) Version() string {
	return "{{.Version}}"
}

func (m *{{.StructName}}) Up(ctx context.Context, tx pgx.Tx) {
	panic("Not implemented")
}

func (m *{{.StructName}}) Down(ctx context.Context, tx pgx.Tx) {
	panic("Not implemented")
}
`
	tmpl := template.Must(template.New("migration").Parse(templateText))

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, templateParams)
	if err != nil {
		panic(err)
	}

	filename := fmt.Sprintf("db/migrations/%s_%s.go", version, name)
	err = os.WriteFile(filename, buf.Bytes(), 0666)
	if err != nil {
		panic(err)
	}

	fmt.Println("Created", filename)
}

func migrate() {
	lockId := migrationLock()
	defer migrationUnlock(lockId)()

	versionsRows, err := Conn.Query(
		context.Background(), "select version from schema_migrations order by version asc",
	)
	if err != nil {
		panic(err)
	}
	dbVersions := make(map[string]bool)
	var latestDbVersion string
	for versionsRows.Next() {
		var version string
		if err := versionsRows.Scan(&version); err != nil {
			panic(err)
		}
		dbVersions[version] = true
		if version > latestDbVersion {
			latestDbVersion = version
		}
	}
	if err := versionsRows.Err(); err != nil {
		panic(err)
	}

	for _, migration := range migrations.All {
		version := migration.Version()
		if version <= latestDbVersion && !dbVersions[version] {
			panic(fmt.Errorf("Old migration is not in db: %s", version))
		}
	}

	ctx := context.Background()
	for _, migration := range migrations.All {
		version := migration.Version()
		if version <= latestDbVersion {
			continue
		}

		tx, err := Conn.Begin(ctx)
		if err != nil {
			panic(err)
		}
		defer func() {
			if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				panic(errors.Wrap(err, "rollback error"))
			}
		}()

		migration.Up(ctx, tx)
		_, err = tx.Exec(ctx, "insert into schema_migrations (version) values ($1)", version)
		if err != nil {
			panic(err)
		}
		if err := tx.Commit(ctx); err != nil {
			panic(err)
		}

		dumpStructure()
		fmt.Println(version)
	}
}

func rollback() {
	lockId := migrationLock()
	defer migrationUnlock(lockId)()

	ctx := context.Background()
	maxVersionRow := Conn.QueryRow(ctx, "select max(version) from schema_migrations")
	var maxVersion string
	if err := maxVersionRow.Scan(&maxVersion); err != nil {
		panic(err)
	}

	found := false
	for _, migration := range migrations.All {
		if migration.Version() != maxVersion {
			continue
		}
		found = true

		tx, err := Conn.Begin(ctx)
		if err != nil {
			panic(err)
		}
		defer func() {
			if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				panic(errors.Wrap(err, "rollback error"))
			}
		}()

		migration.Down(ctx, tx)
		tag, err := tx.Exec(ctx, "delete from schema_migrations where version = $1", maxVersion)
		if err != nil {
			panic(err)
		}
		if tag.RowsAffected() != 1 {
			panic(fmt.Errorf("Expected to delete a single row, got %d", tag.RowsAffected()))
		}
		if err := tx.Commit(ctx); err != nil {
			panic(err)
		}

		dumpStructure()
		fmt.Println(maxVersion, "rolled back")

		break
	}

	if !found {
		panic(fmt.Errorf("Migration version %s not found in code", maxVersion))
	}
}

func migrationLock() int {
	dbNameHash := crc32.ChecksumIEEE([]byte(config.Cfg.DB.DBName))
	const migratorSalt = 2053462845
	lockId := migratorSalt * int(dbNameHash)
	lockRow := Conn.QueryRow(context.Background(), "select pg_try_advisory_lock($1)", lockId)
	var gotLock bool
	err := lockRow.Scan(&gotLock)
	if err != nil {
		panic(err)
	}
	if !gotLock {
		panic("Cannot run migrations because another migration process is currently running")
	}

	return lockId
}

func migrationUnlock(lockId int) func() {
	return func() {
		row := Conn.QueryRow(context.Background(), "select pg_advisory_unlock($1)", lockId)
		var unlocked bool
		err := row.Scan(&unlocked)
		if err != nil {
			panic(err)
		}
		if !unlocked {
			panic("Failed to release advisory lock")
		}
	}
}
