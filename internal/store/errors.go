package store

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Errors the writer and reader return. Callers match them with errors.Is.
var (
	// ErrNotFound means no record with that id is visible to the caller: it
	// does not exist, or it lives in an area the caller cannot read. The two
	// are deliberately indistinguishable (TR-75).
	ErrNotFound = errors.New("store: record not found")
	// ErrForbidden means the caller's areas do not cover the record, or
	// PostgreSQL's row-level security rejected the write.
	ErrForbidden = errors.New("store: caller may not access this area")
	// ErrRevisionConflict means the record changed since the revision the
	// caller based its update on (TR-90).
	ErrRevisionConflict = errors.New("store: record revision has moved")
	// ErrDuplicateEvidence means the area already holds evidence for that
	// source URI, revision and locator; link the existing record instead.
	ErrDuplicateEvidence = errors.New("store: evidence for this source already exists in the area")
	// ErrCrossArea means a record links a record in another area, for
	// example a deduction derived from an assertion outside its own area.
	ErrCrossArea = errors.New("store: a record may only link records in its own area")
	// ErrNoCaller means the caller has no user id.
	ErrNoCaller = errors.New("store: caller has no user id")
)

const (
	sqlstateInsufficientPrivilege = "42501"
	sqlstateUniqueViolation       = "23505"
)

// mapErr turns PostgreSQL and pgx errors into the package's sentinels where
// one applies, keeping the original as the cause.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == sqlstateInsufficientPrivilege:
			return errors.Join(ErrForbidden, err)
		case pgErr.Code == sqlstateUniqueViolation && pgErr.ConstraintName == "evidence_source":
			return errors.Join(ErrDuplicateEvidence, err)
		}
	}
	return err
}
