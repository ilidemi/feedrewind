// pgx wrappers
package pgw

import (
	"context"
	"errors"
	"feedrewind/log"
	"feedrewind/oops"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Queryable interface {
	Begin() (*Tx, error)
	Exec(sql string, args ...any) (pgconn.CommandTag, error)
	Query(sql string, args ...any) (*Rows, error)
	QueryRow(sql string, args ...any) *Row
	NewBatch() *Batch
	SendBatch(batch *Batch) *BatchResults
	Logger() log.Logger
	Context() context.Context
}

type Pool struct {
	impl *pgxpool.Pool
}

func NewPool(ctx context.Context, connString string) (*Pool, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	return &Pool{pool}, nil
}

func (pool *Pool) Acquire(ctx context.Context, logger log.Logger) (*Conn, error) {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	conn, err := pool.impl.Acquire(ctx)
	if err != nil {
		return nil, oops.Wrap(err)
	}

	return &Conn{
		logger: logger,
		impl:   conn,
		ctx:    ctx,
	}, nil
}

func (pool *Pool) AcquireBackground() (*Conn, error) {
	// Do not track duration, as we're not in request context

	ctx := context.Background()
	conn, err := pool.impl.Acquire(ctx)
	if err != nil {
		return nil, oops.Wrap(err)
	}

	return &Conn{
		logger: &log.BackgroundLogger{},
		impl:   conn,
		ctx:    ctx,
	}, nil
}

type Conn struct {
	logger log.Logger
	impl   *pgxpool.Conn
	ctx    context.Context
}

func (conn *Conn) Begin() (*Tx, error) {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	tx, err := conn.impl.Begin(conn.ctx)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	return &Tx{
		logger:    conn.logger,
		impl:      tx,
		ctx:       conn.ctx,
		beginTime: t1,
	}, nil
}

func (conn *Conn) Exec(sql string, args ...any) (pgconn.CommandTag, error) {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	if err := checkDiscarded(sql); err != nil {
		return pgconn.CommandTag{}, err // nolint:exhaustruct
	}

	result, err := conn.impl.Exec(conn.ctx, sql, args...)
	return result, oops.Wrap(err)
}

func (conn *Conn) ExecWithContext(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t1 := time.Now()
	defer addDuration(ctx, t1)()

	if err := checkDiscarded(sql); err != nil {
		return pgconn.CommandTag{}, err // nolint:exhaustruct
	}

	result, err := conn.impl.Exec(ctx, sql, args...)
	return result, oops.Wrap(err)
}

func (conn *Conn) Query(sql string, args ...any) (*Rows, error) {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	if err := checkDiscarded(sql); err != nil {
		return nil, err
	}

	rows, err := conn.impl.Query(conn.ctx, sql, args...)
	return newRows(rows, conn.ctx), oops.Wrap(err)
}

func (conn *Conn) QueryRow(sql string, args ...any) *Row {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	if err := checkDiscarded(sql); err != nil {
		return newErrRow(err)
	}

	row := conn.impl.QueryRow(conn.ctx, sql, args...)
	return newRow(row)
}

func (conn *Conn) NewBatch() *Batch {
	return &Batch{
		impl: pgx.Batch{}, //nolint:exhaustruct
		ctx:  conn.ctx,
	}
}

func (conn *Conn) SendBatch(batch *Batch) *BatchResults {
	t1 := time.Now()
	defer addDuration(conn.ctx, t1)()

	return newBatchResults(conn.impl.SendBatch(conn.ctx, &batch.impl))
}

func (conn *Conn) Logger() log.Logger {
	return conn.logger
}

func (conn *Conn) Context() context.Context {
	return conn.ctx
}

func (conn *Conn) Release() {
	conn.impl.Release()
}

func (conn *Conn) WaitForNotification() (*pgconn.Notification, error) {
	// Do not track duration. We're mostly waiting here and it's by design.

	notification, err := conn.impl.Conn().WaitForNotification(conn.ctx)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	return notification, nil
}

type Tx struct {
	logger    log.Logger
	impl      pgx.Tx
	ctx       context.Context
	beginTime time.Time
}

// Begin starts a pseudo nested transaction.
func (tx *Tx) Begin() (*Tx, error) {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	nested, err := tx.impl.Begin(tx.ctx)
	if err != nil {
		return nil, oops.Wrap(err)
	}

	return &Tx{
		logger:    tx.logger,
		impl:      nested,
		ctx:       tx.ctx,
		beginTime: t1,
	}, nil
}

const longTransactionThreshold = 10 * time.Second

// Commit commits the transaction if this is a real transaction or releases the savepoint if this is a pseudo nested
// transaction. Commit will return an error where errors.Is(ErrTxClosed) is true if the Tx is already closed, but is
// otherwise safe to call multiple times. If the commit fails with a rollback status (e.g. the transaction was already
// in a broken state) then an error where errors.Is(ErrTxCommitRollback) is true will be returned.
func (tx *Tx) Commit() error {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	err := tx.impl.Commit(tx.ctx)
	if txTime := time.Since(tx.beginTime); txTime >= longTransactionThreshold {
		tx.logger.Warn().Msgf("Long running transaction: %v", txTime)
	}
	return oops.Wrap(err)
}

// Rollback rolls back the transaction if this is a real transaction or rolls back to the savepoint if this is a
// pseudo nested transaction. Rollback will return an error where errors.Is(ErrTxClosed) is true if the Tx is already
// closed, but is otherwise safe to call multiple times. Hence, a defer tx.Rollback() is safe even if tx.Commit() will
// be called first in a non-error condition. Any other failure of a real transaction will result in the connection
// being closed.
func (tx *Tx) Rollback() error {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	err := tx.impl.Rollback(tx.ctx)
	if txTime := time.Since(tx.beginTime); txTime >= longTransactionThreshold {
		tx.logger.Warn().Msgf("Long running transaction: %v", txTime)
	}
	return oops.Wrap(err)
}

func (tx *Tx) Exec(sql string, arguments ...any) (pgconn.CommandTag, error) {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	if err := checkDiscarded(sql); err != nil {
		return pgconn.CommandTag{}, err // nolint:exhaustruct
	}

	result, err := tx.impl.Exec(tx.ctx, sql, arguments...)
	return result, oops.Wrap(err)
}

func (tx *Tx) Query(sql string, arguments ...any) (*Rows, error) {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	if err := checkDiscarded(sql); err != nil {
		return nil, err
	}

	rows, err := tx.impl.Query(tx.ctx, sql, arguments...)
	return newRows(rows, tx.ctx), oops.Wrap(err)
}

func (tx *Tx) QueryRow(sql string, arguments ...any) *Row {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	if err := checkDiscarded(sql); err != nil {
		return newErrRow(err)
	}

	row := tx.impl.QueryRow(tx.ctx, sql, arguments...)
	return newRow(row)
}

func (tx *Tx) NewBatch() *Batch {
	return &Batch{
		impl: pgx.Batch{}, //nolint:exhaustruct
		ctx:  tx.ctx,
	}
}

func (tx *Tx) SendBatch(batch *Batch) *BatchResults {
	t1 := time.Now()
	defer addDuration(tx.ctx, t1)()

	return newBatchResults(tx.impl.SendBatch(tx.ctx, &batch.impl))
}

func (tx *Tx) Logger() log.Logger {
	return tx.logger
}

func (tx *Tx) Context() context.Context {
	return tx.ctx
}

type Rows struct {
	impl pgx.Rows
	ctx  context.Context
}

func newRows(rows pgx.Rows, ctx context.Context) *Rows {
	if rows == nil {
		return nil
	}

	return &Rows{
		impl: rows,
		ctx:  ctx,
	}
}

// Next prepares the next row for reading. It returns true if there is another
// row and false if no more rows are available. It automatically closes rows
// when all rows are read.
func (rows *Rows) Next() bool {
	t1 := time.Now()
	defer addDuration(rows.ctx, t1)()

	return rows.impl.Next()
}

// Scan reads the values from the current row into dest values positionally.
// dest can include pointers to core types, values implementing the Scanner
// interface, and nil. nil will skip the value entirely. It is an error to
// call Scan without first calling Next() and checking that it returned true.
func (rows *Rows) Scan(dest ...any) error {
	err := rows.impl.Scan(dest...)
	return oops.Wrap(err)
}

// Err returns any error that occurred while reading. Err must only be called after the Rows is closed (either by
// calling Close or by Next returning false). If it is called early it may return nil even if there was an error
// executing the query.
func (rows *Rows) Err() error {
	err := rows.impl.Err()
	return oops.Wrap(err)
}

// Close closes the rows, making the connection ready for use again. It is safe
// to call Close after rows is already closed.
func (rows *Rows) Close() {
	rows.impl.Close()
}

type Row struct {
	impl pgx.Row
	err  error
}

func newRow(row pgx.Row) *Row {
	if row == nil {
		return nil
	}

	return &Row{impl: row, err: nil}
}

func newErrRow(err error) *Row {
	return &Row{impl: nil, err: err}
}

// Scan works the same as Rows. with the following exceptions. If no
// rows were found it returns ErrNoRows. If multiple rows are returned it
// ignores all but the first.
func (row *Row) Scan(dest ...any) error {
	if row.err != nil {
		return oops.Wrap(row.err)
	}

	err := row.impl.Scan(dest...)
	return oops.Wrap(err)
}

type Batch struct {
	impl pgx.Batch
	ctx  context.Context
}

func NewBatch(ctx context.Context) *Batch {
	return &Batch{
		impl: pgx.Batch{}, // nolint:exhaustruct
		ctx:  ctx,
	}
}

// Queue queues a query to batch b. query can be an SQL query or the name of a prepared statement.
func (b *Batch) Queue(query string, arguments ...any) *QueuedQuery {
	return &QueuedQuery{
		impl: b.impl.Queue(query, arguments...),
		ctx:  b.ctx,
	}
}

type QueuedQuery struct {
	impl *pgx.QueuedQuery
	ctx  context.Context
}

// Query sets fn to be called when the response to qq is received.
func (qq *QueuedQuery) Query(fn func(rows Rows) error) {
	qq.impl.Query(func(rows pgx.Rows) error {
		return fn(*newRows(rows, qq.ctx))
	})
}

// Query sets fn to be called when the response to qq is received.
func (qq *QueuedQuery) QueryRow(fn func(row Row) error) {
	qq.impl.QueryRow(func(row pgx.Row) error {
		return fn(*newRow(row))
	})
}

// Exec sets fn to be called when the response to qq is received.
func (qq *QueuedQuery) Exec(fn func(ct pgconn.CommandTag) error) {
	qq.impl.Exec(fn)
}

type BatchResults struct {
	impl pgx.BatchResults
}

func newBatchResults(impl pgx.BatchResults) *BatchResults {
	return &BatchResults{impl: impl}
}

// Close closes the batch operation. All unread results are read and any callback functions registered with
// QueuedQuery.Query, QueuedQuery.QueryRow, or QueuedQuery.Exec will be called. If a callback function returns an
// error or the batch encounters an error subsequent callback functions will not be called.
//
// Close must be called before the underlying connection can be used again. Any error that occurred during a batch
// operation may have made it impossible to resyncronize the connection with the server. In this case the underlying
// connection will have been closed.
//
// Close is safe to call multiple times. If it returns an error subsequent calls will return the same error. Callback
// functions will not be rerun.
func (r *BatchResults) Close() error {
	return oops.Wrap(r.impl.Close())
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

var fromSubscriptionsRegex *regexp.Regexp
var ErrDontUseSubscriptions = errors.New("Use of subscriptions table is deprecated. Use subscriptions_with_discarded or subscriptions_without_discarded instead.")
var CheckSubscriptionsUsage = true
var fromUsersRegex *regexp.Regexp
var ErrDontUseUsers = errors.New("Use of users table is deprecated. Use users_with_discarded or users_without_discarded instead.")
var CheckUsersUsage = true

func init() {
	fromSubscriptionsRegex = regexp.MustCompile(`\b(from|into)\s+subscriptions\b`)
	fromUsersRegex = regexp.MustCompile(`\b(from|into)\s+users\b`)
}

func checkDiscarded(sql string) error {
	if CheckSubscriptionsUsage && fromSubscriptionsRegex.MatchString(sql) {
		return oops.Wrap(ErrDontUseSubscriptions)
	}

	if CheckUsersUsage && fromUsersRegex.MatchString(sql) {
		return oops.Wrap(ErrDontUseUsers)
	}

	return nil
}
