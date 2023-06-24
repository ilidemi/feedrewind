// pgx wrappers
package pgw

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Queryable interface {
	MustExec(ctx context.Context, sql string, args ...any) pgconn.CommandTag
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Pool struct {
	impl *pgxpool.Pool
}

func (pool *Pool) Begin(ctx context.Context) (*Tx, error) {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	tx, err := pool.impl.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &Tx{tx}, nil
}

func (pool *Pool) MustExec(ctx context.Context, sql string, args ...any) pgconn.CommandTag {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	tag, err := pool.impl.Exec(ctx, sql, args...)
	if err != nil {
		panic(err)
	}
	return tag
}

func (pool *Pool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	return pool.impl.Exec(ctx, sql, args...)
}

func (pool *Pool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	return pool.impl.Query(ctx, sql, args...)
}

func (pool *Pool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	return pool.impl.QueryRow(ctx, sql, args...)
}

func NewPool(ctx context.Context, connString string) (*Pool, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}
	return &Pool{pool}, nil
}

type Tx struct {
	impl pgx.Tx
}

// Begin starts a pseudo nested transaction.
func (tx *Tx) Begin(ctx context.Context) (Tx, error) {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	nested, err := tx.impl.Begin(ctx)
	if err != nil {
		return Tx{nil}, err
	}

	return Tx{nested}, nil
}

// Commit commits the transaction if this is a real transaction or releases the savepoint if this is a pseudo nested
// transaction. Commit will return an error where errors.Is(ErrTxClosed) is true if the Tx is already closed, but is
// otherwise safe to call multiple times. If the commit fails with a rollback status (e.g. the transaction was already
// in a broken state) then an error where errors.Is(ErrTxCommitRollback) is true will be returned.
func (tx *Tx) Commit(ctx context.Context) error {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	return tx.impl.Commit(ctx)
}

// Rollback rolls back the transaction if this is a real transaction or rolls back to the savepoint if this is a
// pseudo nested transaction. Rollback will return an error where errors.Is(ErrTxClosed) is true if the Tx is already
// closed, but is otherwise safe to call multiple times. Hence, a defer tx.Rollback() is safe even if tx.Commit() will
// be called first in a non-error condition. Any other failure of a real transaction will result in the connection
// being closed.
func (tx *Tx) Rollback(ctx context.Context) error {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	return tx.impl.Rollback(ctx)
}

func (tx *Tx) Exec(ctx context.Context, sql string, arguments ...any) (
	commandTag pgconn.CommandTag, err error,
) {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	return tx.impl.Exec(ctx, sql, arguments...)
}

func (tx *Tx) MustExec(ctx context.Context, sql string, arguments ...any) pgconn.CommandTag {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	tag, err := tx.impl.Exec(ctx, sql, arguments...)
	if err != nil {
		panic(err)
	}
	return tag
}

func (tx *Tx) Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error) {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	return tx.impl.Query(ctx, sql, arguments...)
}

func (tx *Tx) QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	return tx.impl.QueryRow(ctx, sql, arguments...)
}

type dbDurationKeyType struct{}

var dbDurationKey = &dbDurationKeyType{}

func addDuration(ctx context.Context, t1 time.Time) func() {
	return func() {
		t2 := time.Now()
		dbDurationAny := ctx.Value(dbDurationKey)
		if dbDurationAny != nil {
			dbDuration := dbDurationAny.(*time.Duration)
			*dbDuration += t2.Sub(t1)
		}
	}
}

func DbDuration(ctx context.Context) time.Duration {
	dbDuration := ctx.Value(dbDurationKey)
	if dbDuration == nil {
		panic("Must call pgw.WithDBDuration() first")
	}

	return *dbDuration.(*time.Duration)
}

func WithDBDuration(r *http.Request) *http.Request {
	dbDuration := time.Duration(0)
	r = r.WithContext(context.WithValue(r.Context(), dbDurationKey, &dbDuration))
	return r
}
