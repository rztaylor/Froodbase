//go:build integration

package apitest

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/wire"
)

// TestEvidenceForExistingClaim adds evidence to something already recorded
// through contribute (FRB-57), as a client sees it: an observation and an
// inference derived from it, one root each and the inference below its
// threshold. A later call cites a new, independent source for the
// observation under links. The observation gains one root and keeps
// revision 1; the result lists the new evidence as written and the
// observation as linked. Retrying the call replays the result and writes
// nothing. The dreamer passes the root on, so the inference meets its
// threshold, still at revision 1. Evidence a claim already counts, directly
// or through its source, adds nothing; an unknown claim or a claim given by
// ref is refused.
func TestEvidenceForExistingClaim(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	bob, carol := env.Client("bob"), env.Client("carol")

	first, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-31-gateway", Rationale: "seen during INC-31",
		Entities: []wire.EntityInput{{Ref: "gw", Type: "service", Names: []wire.Name{{Name: "Edge Gateway"}}}},
		Evidence: []wire.EvidenceInput{{Ref: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/931", SourceRevision: "2"}},
		Assertions: []wire.AssertionInput{
			{Ref: "obs", Kind: "observation", Subject: "gw", BodyMarkdown: "The edge gateway returned 503s during the nightly batch", Evidence: []string{"ticket"}},
			{Ref: "inf", Kind: "inference", Subject: "gw", BodyMarkdown: "The edge gateway is undersized for the nightly batch", DerivedFrom: []string{"obs"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := written(first)
	obs, inf, ticket := refs["obs"], refs["inf"], refs["ticket"]
	if inf.Support == nil || inf.Support.Roots != 1 || inf.Support.MeetsThreshold {
		t.Fatalf("inference written = %+v, want one root below threshold", inf.Support)
	}

	corroborate := wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-31-gateway-logs", Rationale: "gateway logs agree",
		Evidence: []wire.EvidenceInput{{Ref: "logs", SourceURI: "https://github.com/org/edge/blob/0123456789abcdef0123456789abcdef01234567/capacity.md", Locator: "L10-L24"}},
		Links:    []wire.LinkInput{{Claim: obs.ID, Evidence: []string{"logs"}}},
	}
	out, err := bob.Contribute(ctx, corroborate)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Records) != 1 || out.Records[0].Ref != "logs" || out.Records[0].RecordType != "evidence" {
		t.Errorf("records = %+v, want only the new evidence", out.Records)
	}
	logs := out.Records[0].ID
	if len(out.Linked) != 1 {
		t.Fatalf("linked = %+v, want the observation", out.Linked)
	}
	l := out.Linked[0]
	if l.ID != obs.ID || l.Kind != "observation" || l.Revision != 1 || !reflect.DeepEqual(l.Evidence, []string{logs}) || l.RootsAdded != 1 ||
		l.Support == nil || l.Support.Roots != 2 || l.Support.Score != 2 || len(l.Support.RootsBySourceKind) != 2 {
		t.Errorf("linked = %+v (support %+v), want the observation at revision 1 gaining one root to two", l, l.Support)
	}

	replay, err := bob.Contribute(ctx, corroborate)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.ContributionID != out.ContributionID || !reflect.DeepEqual(replay.Linked, out.Linked) || !reflect.DeepEqual(replay.Records, out.Records) {
		t.Errorf("replay = %+v, want the first result replayed", replay)
	}

	rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: obs.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Revision != 1 || rec.Support == nil || rec.Support.Roots != 2 || len(rec.Citations) != 2 {
		t.Errorf("get_record(observation) = revision %d, support %+v, %d citations; want revision 1, two roots, two citations", rec.Revision, rec.Support, len(rec.Citations))
	}
	hist, err := carol.GetHistory(ctx, wire.GetHistoryInput{ID: obs.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Revisions) != 1 || len(hist.SupportEvents) != 2 || hist.SupportEvents[1].Rationale != "gateway logs agree" || hist.SupportEvents[1].ActorUserID != "bob" {
		t.Errorf("get_history(observation) = %d revisions, events %+v; want one revision and bob's second root_added", len(hist.Revisions), hist.SupportEvents)
	}

	WaitFor(t, func() bool {
		rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: inf.ID})
		return err == nil && rec.Support != nil && rec.Support.Roots == 2
	}, 60*time.Second, "the inference gaining the observation's new root")
	rec, err = carol.GetRecord(ctx, wire.GetRecordInput{ID: inf.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Revision != 1 || !rec.Support.MeetsThreshold {
		t.Errorf("get_record(inference) = revision %d, support %+v; want revision 1 meeting its threshold", rec.Revision, rec.Support)
	}

	// The observation already counts the logs; the inference already
	// counts the ticket through the observation. Neither earns anything.
	again, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-31-again",
		Links: []wire.LinkInput{{Claim: obs.ID, Evidence: []string{logs}}, {Claim: inf.ID, Evidence: []string{ticket.ID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Linked) != 2 || again.Linked[0].RootsAdded != 0 || again.Linked[1].RootsAdded != 0 ||
		again.Linked[0].Support.Roots != 2 || again.Linked[1].Support.Roots != 2 || len(again.Records) != 0 {
		t.Errorf("counted evidence = %+v, want nothing added", again)
	}

	for name, link := range map[string]wire.LinkInput{
		"an unknown claim":    {Claim: "6f1c9a52-3b0e-4f4e-9d61-2a7c5e8b1d40", Evidence: []string{"more"}},
		"a ref, not an id":    {Claim: "obs", Evidence: []string{"more"}},
		"evidence not listed": {Claim: obs.ID, Evidence: []string{"logs"}},
	} {
		_, err := bob.Contribute(ctx, wire.ContributeInput{
			Area: "demo", IdempotencyKey: "bob-refused-" + strings.ReplaceAll(name, " ", "-"),
			Evidence: []wire.EvidenceInput{{Ref: "more", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/932", SourceRevision: "1"}},
			Links:    []wire.LinkInput{link},
		})
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: obs.ID}); err != nil || len(rec.Citations) != 2 {
		t.Errorf("after refusals the observation has %d citations, %v; want 2", len(rec.Citations), err)
	}
}
