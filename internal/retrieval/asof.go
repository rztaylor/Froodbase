package retrieval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/contribute"
	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/wire"
)

// ErrNoValidity means get_as_of was asked about a record that has no
// validity of its own: evidence.
var ErrNoValidity = errors.New("retrieval: this record has no validity to ask about")

// GetAsOf reads a record as valid at a time, evaluated on validity time
// (TR-34): a record recorded later but valid earlier is valid then, and one
// recorded before but valid only later is not. An assertion is valid when
// the as_of mode keeps it (its validity includes the time, unknown validity
// included, and it was not retracted); an entity when it is not retracted
// and at least one of its names was valid then, and it is returned with
// those names only. The record is the current revision: what Froodbase
// knows now about the time asked, not what it knew then (get_history reads
// the revisions by recording time).
func (s *Service) GetAsOf(ctx context.Context, ac *authz.Context, in wire.GetAsOfInput) (wire.GetAsOfOutput, error) {
	if ac == nil {
		return wire.GetAsOfOutput{}, authz.ErrNoContext
	}
	id := domain.ID(strings.TrimSpace(in.ID))
	if id == "" {
		return wire.GetAsOfOutput{}, fmt.Errorf("%w: id is required", ErrNotFound)
	}
	if in.AsOf == nil || in.AsOf.IsZero() {
		return wire.GetAsOfOutput{}, fmt.Errorf("%w: as_of is required: the time to read the record at (RFC 3339, for example 2026-09-03T00:00:00Z)", ErrInvalidQuery)
	}
	at := in.AsOf.UTC()
	q := domain.AsOfQuery(at)
	now := s.now().UTC()
	caller, err := contribute.CallerFor(ctx, s.areas, ac)
	if err != nil {
		return wire.GetAsOfOutput{}, err
	}
	out := wire.GetAsOfOutput{ID: string(id), AsOf: at}

	if a, err := s.pool.GetAssertion(ctx, caller, id); err == nil {
		mode, err := s.pool.ClassifyAssertion(ctx, caller, id, q, now)
		if err != nil {
			return wire.GetAsOfOutput{}, err
		}
		if !mode.Included {
			out.Reason = notValidReason(a.LifecycleState, a.Assertion.ValidFrom, a.Assertion.ValidTo, at)
			return out, nil
		}
		rec, err := s.assertionRecord(ctx, caller, a, mode)
		if err != nil {
			return wire.GetAsOfOutput{}, err
		}
		out.Valid, out.Record = true, &rec
		return out, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return wire.GetAsOfOutput{}, err
	}

	if e, err := s.pool.GetEntity(ctx, caller, id); err == nil {
		modes, err := s.pool.ClassifyNames(ctx, caller, id, q, now)
		if err != nil {
			return wire.GetAsOfOutput{}, err
		}
		rec, err := s.entityRecord(ctx, e)
		if err != nil {
			return wire.GetAsOfOutput{}, err
		}
		var names []wire.Name
		for _, m := range modes {
			if m.Included && m.Position < len(rec.Entity.Names) {
				names = append(names, rec.Entity.Names[m.Position])
			}
		}
		if len(names) == 0 {
			if e.LifecycleState == string(domain.LifecycleRetracted) {
				out.Reason = "retracted: a withdrawn record is not valid at any time"
			} else {
				out.Reason = "none of its names was valid at " + at.Format(time.RFC3339) + "; read it with get_record for every name and its validity"
			}
			return out, nil
		}
		rec.Entity.Names = names
		out.Valid, out.Record = true, &rec
		return out, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return wire.GetAsOfOutput{}, err
	}

	if _, err := s.pool.GetEvidence(ctx, caller, id); err == nil {
		return wire.GetAsOfOutput{}, fmt.Errorf("%w: %s is evidence, which records where something was seen and has no validity of its own; read it with get_record, or ask get_as_of about the claims that cite it", ErrNoValidity, id)
	} else if !errors.Is(err, store.ErrNotFound) {
		return wire.GetAsOfOutput{}, err
	}
	return wire.GetAsOfOutput{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// notValidReason says why an assertion was not valid at a time: it was
// withdrawn, or its validity window does not include the time.
func notValidReason(state string, from, to *time.Time, at time.Time) string {
	if state == string(domain.LifecycleRetracted) {
		return "retracted: a withdrawn record is not valid at any time"
	}
	switch {
	case from != nil && at.Before(*from) && to != nil:
		return fmt.Sprintf("valid from %s until %s, which is after %s", from.Format(time.RFC3339), to.Format(time.RFC3339), at.Format(time.RFC3339))
	case from != nil && at.Before(*from):
		return fmt.Sprintf("valid only from %s, which is after %s", from.Format(time.RFC3339), at.Format(time.RFC3339))
	case to != nil:
		return fmt.Sprintf("its validity ended at %s, before or at %s", to.Format(time.RFC3339), at.Format(time.RFC3339))
	}
	return "not valid at " + at.Format(time.RFC3339)
}
