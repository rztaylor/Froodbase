//go:build integration

package apitest

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/rztaylor/froodbase/internal/wire"
)

// TestDeductionFromTwoObservations is the slice 1b deduction scenario
// (FRB-49), driven through internal/client as froodbase-mcp drives the API:
// an agent records two observations, reads them back, and contributes an
// inference derived from both. The inference is active, kind inference,
// derived from the two observations, and inherits their distinct roots: the
// observations cite the same ticket (one directly, one through a quote of
// it), so the inference has one root and stays below the inference threshold,
// yet get_record and search still return it with the verdict false.
// Reading it, or recording the same conclusion again, earns it nothing;
// deriving from an unknown id, or without any grounds, is refused.
func TestDeductionFromTwoObservations(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	bob, carol := env.Client("bob"), env.Client("carol")

	// Two observations: the slice 1 observations form and the assertions
	// form, citing one root between them.
	first, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "deduce-1", Rationale: "seen during INC-7", InvestigationRef: "INC-7",
		Entities: []wire.EntityInput{{Ref: "tool", Type: "service", Names: []wire.Name{{Name: "Import Tool"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "ticket", SourceKind: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/907", SourceRevision: "3", AccessClass: "area"},
			{Ref: "quote", SourceKind: "document", SourceURI: "https://contoso.sharepoint.com/sites/eng/inc-7.docx", SourceRevision: "v2", Locator: "timeline", AccessClass: "area", Root: "ticket"},
		},
		Observations: []wire.ObservationInput{{Ref: "slow", Subject: "tool", BodyMarkdown: "Import callbacks took over 30 seconds during the nightly batch", Evidence: []string{"ticket"}}},
		Assertions:   []wire.AssertionInput{{Ref: "queue", Kind: "observation", Subject: "tool", BodyMarkdown: "The callback queue held 40,000 messages at the same time", Evidence: []string{"quote"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := written(first)
	tool, slow, queue := refs["tool"], refs["slow"], refs["queue"]
	for _, o := range []wire.Written{slow, queue} {
		if o.Kind != "observation" || o.Support == nil || o.Support.Roots != 1 || !o.Support.MeetsThreshold {
			t.Fatalf("observation = %+v", o)
		}
	}

	// The agent reads both before concluding anything.
	var roots []string
	for _, id := range []string{slow.ID, queue.ID} {
		rec, err := bob.GetRecord(ctx, wire.GetRecordInput{ID: id})
		if err != nil || rec.Kind != "observation" || rec.Derived || len(rec.DerivedFrom) != 0 {
			t.Fatalf("get_record(%s) = %+v, %v", id, rec, err)
		}
		hist, err := bob.GetHistory(ctx, wire.GetHistoryInput{ID: id})
		if err != nil || len(hist.SupportEvents) != 1 || len(hist.Derivations) != 0 {
			t.Fatalf("get_history(%s) = %+v, %v", id, hist, err)
		}
		roots = append(roots, hist.SupportEvents[0].EvidenceRootIDs...)
	}
	if roots[0] != roots[1] {
		t.Fatalf("observation roots = %v, want the one ticket", roots)
	}

	// The inference, derived from both.
	conclude := func(key string) wire.Written {
		t.Helper()
		out, err := bob.Contribute(ctx, wire.ContributeInput{
			Area: "demo", IdempotencyKey: key, Rationale: "concluded during INC-7",
			Assertions: []wire.AssertionInput{{
				Ref: "cause", Kind: "inference", Subject: tool.ID, DerivedFrom: []string{slow.ID, queue.ID},
				BodyMarkdown: "Slow import callbacks are caused by the callback queue backing up under batch load",
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return written(out)["cause"]
	}
	inf := conclude("deduce-2")
	if inf.Kind != "inference" || inf.LifecycleState != "active" || inf.Support == nil || inf.Support.Roots != 1 || inf.Support.Score != 1 || inf.Support.MeetsThreshold {
		t.Fatalf("inference written = %+v (support %+v), want active, one root, below threshold", inf, inf.Support)
	}

	// Read back by someone else in the area: kind, derived, both sources,
	// the sources' root, below threshold but returned.
	sources := sortedIDs(slow.ID, queue.ID)
	rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: inf.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != "inference" || rec.LifecycleState != "active" || !rec.Derived || !reflect.DeepEqual(sortedIDs(rec.DerivedFrom...), sources) ||
		rec.Support == nil || rec.Support.Roots != 1 || rec.Support.MeetsThreshold || len(rec.Citations) != 0 {
		t.Fatalf("get_record(inference) = %+v (support %+v)", rec, rec.Support)
	}
	hist, err := carol.GetHistory(ctx, wire.GetHistoryInput{ID: inf.ID})
	if err != nil {
		t.Fatal(err)
	}
	var histSources []string
	for _, d := range hist.Derivations {
		if d.Rationale != "concluded during INC-7" || d.RecordedAt.IsZero() {
			t.Errorf("derivation %+v", d)
		}
		histSources = append(histSources, d.SourceID)
	}
	if len(hist.Revisions) != 1 || !reflect.DeepEqual(sortedIDs(histSources...), sources) || len(hist.SupportEvents) != 1 {
		t.Fatalf("get_history(inference) = %+v", hist)
	}
	if e := hist.SupportEvents[0]; e.Type != "root_added" || !reflect.DeepEqual(e.EvidenceRootIDs, roots[:1]) || e.Rationale != "inherited from "+strings.Join(sources, ", ") {
		t.Fatalf("inherited support event = %+v, want root %s inherited from both observations", e, roots[0])
	}
	found, err := carol.Search(ctx, wire.SearchInput{Query: "backing up"})
	if err != nil {
		t.Fatal(err)
	}
	var hit *wire.Record
	for i := range found.Results {
		if found.Results[i].Record.ID == inf.ID {
			hit = &found.Results[i].Record
		}
	}
	if hit == nil || hit.Kind != "inference" || !hit.Derived || hit.Support == nil || hit.Support.MeetsThreshold {
		t.Fatalf("search did not return the inference below threshold: %+v", found)
	}

	// Being read, or concluded again, earns it nothing.
	again := conclude("deduce-3")
	if again.ID == inf.ID || again.Support.Score != 1 {
		t.Fatalf("second conclusion = %+v", again)
	}
	after, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: inf.ID})
	if err != nil || after.Support.Score != rec.Support.Score || after.Support.Events != 1 {
		t.Fatalf("after reads and a repeat: support %+v, %v; want unchanged", after.Support, err)
	}

	// Refusals write nothing.
	count := func() int {
		var n int
		if err := env.DB.Admin.Pgx().QueryRow(ctx, "select (select count(*) from assertion) + (select count(*) from derivation)").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	for name, a := range map[string]wire.AssertionInput{
		"unknown id":          {Ref: "x", Kind: "inference", Subject: tool.ID, BodyMarkdown: "From nowhere", DerivedFrom: []string{slow.ID, "00000000-0000-0000-0000-000000000000"}},
		"no grounds":          {Ref: "x", Kind: "decision", Subject: tool.ID, BodyMarkdown: "Scale the queue"},
		"derived observation": {Ref: "x", Kind: "observation", Subject: tool.ID, BodyMarkdown: "Seen", Evidence: []string{refs["ticket"].ID}, DerivedFrom: []string{slow.ID}},
	} {
		_, err := bob.Contribute(ctx, wire.ContributeInput{Area: "demo", IdempotencyKey: "refused-" + name, Assertions: []wire.AssertionInput{a}})
		if err == nil || !strings.Contains(err.Error(), "invalid change set") {
			t.Errorf("%s: err = %v, want an invalid change set", name, err)
		}
	}
	if after := count(); after != before {
		t.Errorf("refused contributions wrote %d rows", after-before)
	}
}

func written(out wire.ContributeOutput) map[string]wire.Written {
	m := map[string]wire.Written{}
	for _, r := range out.Records {
		m[r.Ref] = r
	}
	return m
}

func sortedIDs(ids ...string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}
