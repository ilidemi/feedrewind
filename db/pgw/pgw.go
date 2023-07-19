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
	MustBegin() *Tx
	Begin() (*Tx, error)
	MustExec(sql string, args ...any) pgconn.CommandTag
	Exec(sql string, args ...any) (pgconn.CommandTag, error)
	Query(sql string, args ...any) (pgx.Rows, error)
	QueryRow(sql string, args ...any) pgx.Row
	SendBatch(batch *pgx.Batch) pgx.BatchResults
}

type Pool struct {
	impl *pgxpool.Pool
}

func NewPool(ctx context.Context, connString string) (*Pool, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}
	return &Pool{pool}, nil
}

func (pool *Pool) Acquire(ctx context.Context) (*Conn, error) {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	conn, err := pool.impl.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	return &Conn{
		impl: conn,
		ctx:  ctx,
	}, nil
}

type Conn struct {
	impl *pgxpool.Conn
	ctx  context.Context
}

func (conn *Conn) MustBegin() *Tx {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	tx, err := conn.impl.Begin(conn.ctx)
	if err != nil {
		panic(err)
	}
	return &Tx{
		impl: tx,
		ctx:  conn.ctx,
	}
}

func (conn *Conn) Begin() (*Tx, error) {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	tx, err := conn.impl.Begin(conn.ctx)
	if err != nil {
		return nil, err
	}
	return &Tx{
		impl: tx,
		ctx:  conn.ctx,
	}, nil
}

func (conn *Conn) MustExec(sql string, args ...any) pgconn.CommandTag {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	tag, err := conn.impl.Exec(conn.ctx, sql, args...)
	if err != nil {
		panic(err)
	}
	return tag
}

func (conn *Conn) Exec(sql string, args ...any) (pgconn.CommandTag, error) {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	return conn.impl.Exec(conn.ctx, sql, args...)
}

func (conn *Conn) Query(sql string, args ...any) (pgx.Rows, error) {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	return conn.impl.Query(conn.ctx, sql, args...)
}

func (conn *Conn) QueryRow(sql string, args ...any) pgx.Row {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	return conn.impl.QueryRow(conn.ctx, sql, args...)
}

func (conn *Conn) SendBatch(batch *pgx.Batch) pgx.BatchResults {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	return conn.impl.SendBatch(conn.ctx, batch)
}

func (conn *Conn) Release() {
	conn.impl.Release()
}

type Tx struct {
	impl pgx.Tx
	ctx  context.Context
}

// MustBegin starts a pseudo nested transaction.
func (tx *Tx) MustBegin() *Tx {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	nested, err := tx.impl.Begin(tx.ctx)
	if err != nil {
		panic(err)
	}

	return &Tx{
		impl: nested,
		ctx:  tx.ctx,
	}
}

// Begin starts a pseudo nested transaction.
func (tx *Tx) Begin() (*Tx, error) {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	nested, err := tx.impl.Begin(tx.ctx)
	if err != nil {
		return nil, err
	}

	return &Tx{
		impl: nested,
		ctx:  tx.ctx,
	}, nil
}

// Commit commits the transaction if this is a real transaction or releases the savepoint if this is a pseudo nested
// transaction. Commit will return an error where errors.Is(ErrTxClosed) is true if the Tx is already closed, but is
// otherwise safe to call multiple times. If the commit fails with a rollback status (e.g. the transaction was already
// in a broken state) then an error where errors.Is(ErrTxCommitRollback) is true will be returned.
func (tx *Tx) Commit() error {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	return tx.impl.Commit(tx.ctx)
}

// Rollback rolls back the transaction if this is a real transaction or rolls back to the savepoint if this is a
// pseudo nested transaction. Rollback will return an error where errors.Is(ErrTxClosed) is true if the Tx is already
// closed, but is otherwise safe to call multiple times. Hence, a defer tx.Rollback() is safe even if tx.Commit() will
// be called first in a non-error condition. Any other failure of a real transaction will result in the connection
// being closed.
func (tx *Tx) Rollback() error {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	return tx.impl.Rollback(tx.ctx)
}

func (tx *Tx) Exec(sql string, arguments ...any) (
	commandTag pgconn.CommandTag, err error,
) {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	return tx.impl.Exec(tx.ctx, sql, arguments...)
}

func (tx *Tx) MustExec(sql string, arguments ...any) pgconn.CommandTag {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	tag, err := tx.impl.Exec(tx.ctx, sql, arguments...)
	if err != nil {
		panic(err)
	}
	return tag
}

func (tx *Tx) Query(sql string, arguments ...any) (pgx.Rows, error) {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	return tx.impl.Query(tx.ctx, sql, arguments...)
}

func (tx *Tx) QueryRow(sql string, arguments ...any) pgx.Row {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	return tx.impl.QueryRow(tx.ctx, sql, arguments...)
}

func (tx *Tx) SendBatch(batch *pgx.Batch) pgx.BatchResults {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	return tx.impl.SendBatch(tx.ctx, batch)
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
