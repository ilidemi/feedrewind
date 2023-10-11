package db

import (
	"bytes"
	"context"
	"feedrewind/config"
	"feedrewind/db/migrations"
	"feedrewind/db/pgw"
	"feedrewind/oops"
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

var Pool *pgw.Pool

func init() {
	var err error
	Pool, err = pgw.NewPool(context.Background(), config.Cfg.DB.DSN())
	if err != nil {
		panic(err)
	}
}

func EnsureLatestMigration() error {
	conn, err := Pool.AcquireBackground()
	if err != nil {
		return err
	}
	defer conn.Release()

	row := conn.QueryRow("select version from schema_migrations order by version desc limit 1")
	var latestDbVersion string
	err = row.Scan(&latestDbVersion)
	if err != nil {
		return err
	}

	for _, migration := range migrations.All {
		version := migration.Version()
		if version > latestDbVersion {
			return oops.Newf("Migration is not in db: %s", version)
		}
	}

	return nil
}

func ensurePgDump() {
	_, err := exec.LookPath("pg_dump")
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
		pgDump, "--schema-only", "--no-privileges", "--no-owner", "--file", filename,
		"--host", config.Cfg.DB.Host, config.Cfg.DB.DBName,
	)
	pgDumpCmd.Stdout = os.Stdout
	pgDumpCmd.Stderr = os.Stderr
	err = pgDumpCmd.Run()
	if err != nil {
		panic(err)
	}

	conn, err := Pool.AcquireBackground()
	if err != nil {
		panic(err)
	}
	defer conn.Release()
	rows, err := conn.Query("select version from schema_migrations order by version asc")
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

type {{.StructName}} struct {}

func init() {
	registerMigration(&{{.StructName}}{})
}

func (m *{{.StructName}}) Version() string {
	return "{{.Version}}"
}

func (m *{{.StructName}}) Up(tx *Tx) {
	panic("Not implemented")
}

func (m *{{.StructName}}) Down(tx *Tx) {
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
	conn, err := Pool.AcquireBackground()
	if err != nil {
		panic(err)
	}
	defer conn.Release()

	lockId := migrationLock(conn)
	defer migrationUnlock(conn, lockId)()

	versionsRows, err := conn.Query("select version from schema_migrations order by version asc")
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

	ensurePgDump()

	for _, migration := range migrations.All {
		version := migration.Version()
		if version <= latestDbVersion {
			continue
		}

		tx, err := conn.Begin()
		if err != nil {
			panic(err)
		}
		defer func() {
			if err := tx.Rollback(); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				panic(errors.Wrap(err, "rollback error"))
			}
		}()

		migration.Up(migrations.WrapTx(tx))
		_, err = tx.Exec("insert into schema_migrations (version) values ($1)", version)
		if err != nil {
			panic(err)
		}

		if err := tx.Commit(); err != nil {
			panic(err)
		}

		dumpStructure()
		fmt.Println(version)
	}
}

func rollback() {
	conn, err := Pool.AcquireBackground()
	if err != nil {
		panic(err)
	}
	defer conn.Release()

	lockId := migrationLock(conn)
	defer migrationUnlock(conn, lockId)()

	maxVersionRow := conn.QueryRow("select max(version) from schema_migrations")
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

		tx, err := conn.Begin()
		if err != nil {
			panic(err)
		}
		defer func() {
			if err := tx.Rollback(); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				panic(errors.Wrap(err, "rollback error"))
			}
		}()

		migration.Down(migrations.WrapTx(tx))
		tag, err := tx.Exec("delete from schema_migrations where version = $1", maxVersion)
		if err != nil {
			panic(err)
		}
		if tag.RowsAffected() != 1 {
			panic(fmt.Errorf("Expected to delete a single row, got %d", tag.RowsAffected()))
		}
		if err := tx.Commit(); err != nil {
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

func migrationLock(conn *pgw.Conn) int {
	dbNameHash := crc32.ChecksumIEEE([]byte(config.Cfg.DB.DBName))
	const migratorSalt = 2053462845
	lockId := migratorSalt * int(dbNameHash)
	lockRow := conn.QueryRow("select pg_try_advisory_lock($1)", lockId)
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

func migrationUnlock(conn *pgw.Conn, lockId int) func() {
	return func() {
		row := conn.QueryRow("select pg_advisory_unlock($1)", lockId)
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
