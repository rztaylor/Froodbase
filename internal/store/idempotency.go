package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rztaylor/froodbase/internal/store/gen"
)

// Idempotency identifies a retryable call (TR-15): the client's key, the
// tool it was for and a hash of the request. The writer stores the result
// under it in the same transaction as the write, so a retry finds either the
// whole result or nothing.
type Idempotency struct {
	Key         string
	Tool        string
	RequestHash string
}

// IdempotentResult is what a stored key returns.
type IdempotentResult struct {
	Tool        string
	RequestHash string
	Response    json.RawMessage
	CreatedAt   time.Time
}

// ErrIdempotencyMismatch means the key was used before with a different
// request; the caller must choose a new key.
var ErrIdempotencyMismatch = errors.New("store: idempotency key was used with a different request")

// ErrIdempotencyRace means another call with the same key committed first;
// the caller should read the stored result.
var ErrIdempotencyRace = errors.New("store: idempotency key was stored concurrently")

// IdempotentResult looks up a stored result for the caller's key. It returns
// ErrNotFound when the key is unused.
func (p *Pool) IdempotentResult(ctx context.Context, caller Caller, key string) (IdempotentResult, error) {
	var out IdempotentResult
	err := p.inTx(ctx, caller, func(ctx context.Context, _ pgx.Tx, q *gen.Queries) error {
		row, err := q.GetIdempotencyKey(ctx, gen.GetIdempotencyKeyParams{CallerUserID: caller.UserID, Key: key})
		if err != nil {
			return err
		}
		out = IdempotentResult{Tool: row.Tool, RequestHash: row.RequestHash, Response: json.RawMessage(row.Response), CreatedAt: row.CreatedAt}
		return nil
	})
	return out, err
}

// storeIdempotent inserts the key with the response inside the caller's
// transaction.
func storeIdempotent(ctx context.Context, q *gen.Queries, caller Caller, idem Idempotency, response any, now time.Time) error {
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	err = q.InsertIdempotencyKey(ctx, gen.InsertIdempotencyKeyParams{
		CallerUserID: caller.UserID, Key: idem.Key, Tool: idem.Tool, RequestHash: idem.RequestHash, Response: body, CreatedAt: now,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlstateUniqueViolation {
		return ErrIdempotencyRace
	}
	return err
}
