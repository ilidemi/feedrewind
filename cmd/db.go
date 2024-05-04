package cmd

import (
	"bytes"
	"feedrewind/config"
	"feedrewind/db"
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

var Db *cobra.Command

func init() {
	Db = &cobra.Command{
		Use: "db",
	}

	dumpCmd := &cobra.Command{
		Use: "dump",
		Run: func(_ *cobra.Command, _ []string) {
			dumpStructure()
		},
	}

	generateMigraionCmd := &cobra.Command{
		Use:     "generate-migration [name]",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"gm"},
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

	Db.AddCommand(dumpCmd)
	Db.AddCommand(generateMigraionCmd)
	Db.AddCommand(migrateCmd)
	Db.AddCommand(rollbackCmd)
}

func ensurePgDump() {
	if config.Cfg.Env.IsDevOrTest() {
		_, err := exec.LookPath("pg_dump")
		if err != nil {
			panic(err)
		}
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

	conn, err := db.Pool.AcquireBackground()
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

	version := time.Now().Format("20060102150405")
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
	conn, err := db.Pool.AcquireBackground()
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
		if !checkIncompleteTables(tx) {
			fmt.Println("Found incomplete tables")
			// panic("Found incomplete tables")
		}
		_, err = tx.Exec("insert into schema_migrations (version) values ($1)", version)
		if err != nil {
			panic(err)
		}

		if err := tx.Commit(); err != nil {
			panic(err)
		}

		if config.Cfg.Env.IsDevOrTest() {
			dumpStructure()
		}
		fmt.Println(version)
	}
}

func rollback() {
	conn, err := db.Pool.AcquireBackground()
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
		if !checkIncompleteTables(tx) {
			fmt.Println("Found incomplete tables")
			// panic("Found incomplete tables")
		}
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

		if config.Cfg.Env.IsDevOrTest() {
			dumpStructure()
		}
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

func checkIncompleteTables(tx *pgw.Tx) bool {
	foundIncompleteTables := false

	for _, column := range []string{"created_at", "updated_at"} {
		rows, err := tx.Query(`
			select table_name from information_schema.tables
			where table_schema = 'public' and
				table_type = 'BASE TABLE' and
				table_name not in (
					select table_name from information_schema.columns where column_name = '` + column + `'
				)
		`)
		if err != nil {
			panic(err)
		}
		for rows.Next() {
			var tableName string
			err := rows.Scan(&tableName)
			if err != nil {
				panic(err)
			}
			fmt.Printf("%s.%s is missing\n", tableName, column)
			foundIncompleteTables = true
		}
		if err := rows.Err(); err != nil {
			panic(err)
		}

		rows, err = tx.Query(`
			select tables.table_name, data_type, is_nullable, column_default
			from information_schema.tables
			left outer join (
				select * from information_schema.columns where column_name = '` + column + `'
			) as columns on tables.table_name = columns.table_name
			where tables.table_schema = 'public' and
				tables.table_type = 'BASE TABLE' and (
					data_type != 'timestamp without time zone' or
					is_nullable != 'NO' or
					column_default != 'utc_now()'
				)
		`)
		if err != nil {
			panic(err)
		}
		for rows.Next() {
			var tableName, dataType, isNullable, columnDefault string
			err := rows.Scan(&tableName, &dataType, &isNullable, &columnDefault)
			if err != nil {
				panic(err)
			}
			if dataType != "timestamp without time zone" {
				fmt.Printf(
					"%s.%s: expected data_type = 'timestamp without time zone', found '%s'\n",
					tableName, column, dataType,
				)
			}
			if isNullable != "NO" {
				fmt.Printf(
					"%s.%s: expected is_nullable = 'NO', found '%s'\n",
					tableName, column, isNullable,
				)
			}
			if columnDefault != "utc_now()" {
				fmt.Printf(
					"%s.%s: expected column_default = 'utc_now()', found '%s'\n",
					tableName, column, columnDefault,
				)
			}
			foundIncompleteTables = true
		}
		if err := rows.Err(); err != nil {
			panic(err)
		}
	}

	rows, err := tx.Query(`
		select table_name from information_schema.tables
		where table_schema = 'public' and
			table_type = 'BASE TABLE' and
			table_name not in (
				select event_object_table from information_schema.triggers
				where trigger_name = 'bump_updated_at'
			)
	`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var tableName string
		err := rows.Scan(&tableName)
		if err != nil {
			panic(err)
		}
		fmt.Printf("bump_updated_at trigger is missing for %s\n", tableName)
		foundIncompleteTables = true
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	rows, err = tx.Query(`
		select tables.table_name, action_timing, event_manipulation, action_orientation, action_statement
		from information_schema.tables
		left outer join (
			select * from information_schema.triggers where trigger_name = 'bump_updated_at'
		) as triggers on tables.table_name = triggers.event_object_table
		where tables.table_schema = 'public' and
			tables.table_type = 'BASE TABLE' and (
				action_timing != 'BEFORE' or
				event_manipulation != 'UPDATE' or
				action_orientation != 'ROW' or
				action_statement != 'EXECUTE FUNCTION bump_updated_at_utc()'
			)
	`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var tableName, actionTiming, eventManipulation, actionOrientation, actionStatement string
		err := rows.Scan(&tableName, &actionTiming, &eventManipulation, &actionOrientation, &actionStatement)
		if err != nil {
			panic(err)
		}
		if actionTiming != "BEFORE" {
			fmt.Printf(
				"%s bump_updated_at: expected action_timing = 'BEFORE', found '%s'\n",
				tableName, actionTiming,
			)
		}
		if eventManipulation != "UPDATE" {
			fmt.Printf(
				"%s bump_updated_at: expected event_manipulation = 'UPDATE', found '%s'\n",
				tableName, eventManipulation,
			)
		}
		if actionOrientation != "ROW" {
			fmt.Printf(
				"%s bump_updated_at: expected action_orientation = 'ROW', found '%s'\n",
				tableName, actionOrientation,
			)
		}
		if actionStatement != "EXECUTE FUNCTION bump_updated_at_utc()" {
			fmt.Printf(
				"%s bump_updated_at: expected action_statement = 'EXECUTE FUNCTION bump_updated_at_utc()', found '%s'\n",
				tableName, actionStatement,
			)
		}
		foundIncompleteTables = true
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	return !foundIncompleteTables
}
