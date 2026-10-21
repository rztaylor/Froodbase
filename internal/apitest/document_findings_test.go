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

// The sample document of the development guide's worked example, cited at a
// commit. The SHA is a stand-in: a real run cites the commit that holds the
// file (see "Recording the findings in a document" in
// docs/dev/ops/development.md).
const (
	sampleCommit = "5a3c1e0b9d8f7a6b5c4d3e2f1a0b9c8d7e6f5a4b"
	sampleDoc    = "https://github.com/rztaylor/Froodbase/blob/" + sampleCommit + "/docs/dev/guides/sample-design-note.md"
)

// TestDocumentFindings is the FRB-53 scenario, driven through
// internal/client as froodbase-mcp drives the API: an agent asked to record
// the findings in docs/dev/guides/sample-design-note.md contributes them as
// the froodbase-contribute skill teaches. The document is never sent; each
// finding cites its section at a commit.
//
// The area already knows the import tool and the work item's finding, so
// the agent searches first: the change set uses the existing entity, leaves
// the Background section's finding alone (it restates the work item) and
// derives from the existing observation. One change set records the
// document once with no locator, as the root of every section it cites, so
// the document counts as one source however many sections are cited:
// observations from Measurements, inferences from Analysis derived from what
// they rest on, and the decision with the date the document states. Open
// questions are not findings and are not sent. Reading back shows each
// claim's section, commit and root. A later call from a person's local notes
// cites the work item the notes name and the person's own statement; the
// local file itself is refused as a citation and nothing is written.
func TestDocumentFindings(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	bob, carol := env.Client("bob"), env.Client("carol")

	// What the area already holds: walkthrough step 4.
	known, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-import-callbacks", Rationale: "found during INC-12",
		Entities: []wire.EntityInput{{Ref: "tool", Type: "service", Names: []wire.Name{{Name: "Import Tool"}}}},
		Evidence: []wire.EvidenceInput{{Ref: "wi", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/418", SourceRevision: "12"}},
		Assertions: []wire.AssertionInput{{Ref: "fails", Kind: "observation", Subject: "tool",
			BodyMarkdown: "Callbacks on 2.3 fail after a 30 second timeout", Evidence: []string{"wi"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tool, fails := written(known)["tool"].ID, written(known)["fails"].ID

	// Search before creating: the import tool and the Background finding are
	// known, the callback gateway is not.
	if got := searchIDs(t, bob, "Import Tool"); len(got) == 0 || got[0] != tool {
		t.Fatalf("search(Import Tool) = %v, want the entity %s first", got, tool)
	}
	if got := searchIDs(t, bob, "callbacks timeout"); !contains(got, fails) {
		t.Fatalf("search(callbacks timeout) = %v, want the recorded observation %s", got, fails)
	}
	found, err := bob.Search(ctx, wire.SearchInput{Query: "Callback Gateway", Area: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range found.Results {
		if r.Record.RecordType == "entity" {
			t.Fatalf("search(Callback Gateway) found entity %+v before it was recorded", r.Record)
		}
	}

	agreed := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	request := wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-sample-design-note", Rationale: "findings of the sample design note",
		Entities: []wire.EntityInput{{Ref: "gateway", Type: "service", Names: []wire.Name{{Name: "Callback Gateway"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "doc", SourceURI: sampleDoc},
			{Ref: "measurements", SourceURI: sampleDoc, Locator: "Measurements", Root: "doc"},
			{Ref: "analysis", SourceURI: sampleDoc, Locator: "Analysis", Root: "doc"},
			{Ref: "decision", SourceURI: sampleDoc, Locator: "Decision", Root: "doc",
				Excerpt: "From version 2.3.2 the callback timeout is capped at 5 seconds"},
		},
		Assertions: []wire.AssertionInput{
			{Ref: "measured", Kind: "observation", Subject: tool, Evidence: []string{"measurements"},
				BodyMarkdown: "In staging, callbacks on 2.3.0 took 30.2 seconds on average over 200 imports, against 1.1 seconds on 2.2.4",
				Scope:        &wire.Scope{Environment: "staging", VersionMin: "2.3.0", VersionMax: "2.3.0"}},
			{Ref: "attempts", Kind: "observation", Subject: "gateway", Evidence: []string{"measurements"},
				BodyMarkdown: "The callback gateway gives up on a callback after three failed attempts"},
			{Ref: "cause", Kind: "inference", Subject: tool, Evidence: []string{"analysis"}, DerivedFrom: []string{fails, "measured"},
				BodyMarkdown: "The 2.3 retry waits out the full 30 second timeout before each retry"},
			{Ref: "drops", Kind: "inference", Subject: "gateway", Evidence: []string{"analysis"}, DerivedFrom: []string{"measured", "attempts"},
				BodyMarkdown: "Slow retries on import tool 2.3 probably make the callback gateway drop callbacks under load"},
			{Ref: "cap", Kind: "decision", Subject: tool, Evidence: []string{"decision"}, DerivedFrom: []string{"cause"}, ValidFrom: &agreed,
				BodyMarkdown: "From 2.3.2 the callback timeout is capped at 5 seconds and retries back off from 1 second",
				Scope:        &wire.Scope{VersionMin: "2.3.2"}},
		},
	}
	out, err := bob.Contribute(ctx, request)
	if err != nil {
		t.Fatalf("document findings refused: %v", err)
	}
	refs := written(out)
	doc := refs["doc"].ID

	// The document is one source: claims resting only on it have one root,
	// and only the deduction that also rests on the work item has two.
	for ref, want := range map[string]struct {
		kind  string
		roots int
		meets bool
	}{
		"measured": {"observation", 1, true},
		"attempts": {"observation", 1, true},
		"cause":    {"inference", 2, true},
		"drops":    {"inference", 1, false},
		"cap":      {"decision", 2, true},
	} {
		w := refs[ref]
		if w.Kind != want.kind || w.Support == nil || w.Support.Roots != want.roots || w.Support.MeetsThreshold != want.meets {
			t.Errorf("%s written = kind %q support %+v; want %s with %d roots, meets threshold %v", ref, w.Kind, w.Support, want.kind, want.roots, want.meets)
		}
	}
	if len(out.Records) != 10 {
		t.Errorf("wrote %d records, want 10 (one entity, four evidence, five claims): %+v", len(out.Records), out.Records)
	}

	// Carol reads each claim back with the section it came from, at the commit.
	for ref, want := range map[string]struct {
		subject, section string
		derivedFrom      []string
	}{
		"measured": {tool, "Measurements", nil},
		"attempts": {refs["gateway"].ID, "Measurements", nil},
		"cause":    {tool, "Analysis", sortedIDs(fails, refs["measured"].ID)},
		"drops":    {refs["gateway"].ID, "Analysis", sortedIDs(refs["measured"].ID, refs["attempts"].ID)},
		"cap":      {tool, "Decision", []string{refs["cause"].ID}},
	} {
		rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: refs[ref].ID})
		if err != nil {
			t.Fatal(err)
		}
		if rec.Subject != want.subject {
			t.Errorf("%s subject = %s, want %s", ref, rec.Subject, want.subject)
		}
		if got := sortedIDs(rec.DerivedFrom...); len(want.derivedFrom) > 0 && !reflect.DeepEqual(got, want.derivedFrom) || len(want.derivedFrom) == 0 && rec.Derived {
			t.Errorf("%s derived_from = %v, want %v", ref, rec.DerivedFrom, want.derivedFrom)
		}
		if len(rec.Citations) != 1 {
			t.Fatalf("%s citations = %+v, want its section", ref, rec.Citations)
		}
		c := rec.Citations[0]
		if c.SourceURI != sampleDoc || c.SourceRevision != sampleCommit || c.SourceKind != "repository" || c.Locator != want.section || c.RootEvidenceID != doc || c.AccessClass != "area" {
			t.Errorf("%s citation = %+v; want %s at %s, section %s, root %s", ref, c, sampleDoc, sampleCommit, want.section, doc)
		}
	}
	decided, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: refs["cap"].ID})
	if err != nil {
		t.Fatal(err)
	}
	if decided.ValidFrom == nil || !decided.ValidFrom.Equal(agreed) || decided.Citations[0].Excerpt == "" {
		t.Errorf("decision valid_from = %v, excerpt %q; want the date the document states and the quoted line", decided.ValidFrom, decided.Citations[0].Excerpt)
	}
	hist, err := carol.GetHistory(ctx, wire.GetHistoryInput{ID: refs["drops"].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.SupportEvents) != 1 || !reflect.DeepEqual(hist.SupportEvents[0].EvidenceRootIDs, []string{doc}) {
		t.Errorf("drops support events = %+v, want one root_added for the document", hist.SupportEvents)
	}
	if got := searchIDs(t, carol, "Callback Gateway"); len(got) == 0 || got[0] != refs["gateway"].ID {
		t.Errorf("search(Callback Gateway) = %v, want the new entity %s first", got, refs["gateway"].ID)
	}

	again, err := bob.Contribute(ctx, request)
	if err != nil || !again.Replayed || !reflect.DeepEqual(again.Records, out.Records) {
		t.Fatalf("retry = %+v, %v; want the first result replayed", again, err)
	}

	// A local file is never a citation, and a refusal writes nothing.
	count := func() int {
		var n int
		if err := env.DB.Admin.Pgx().QueryRow(ctx, "select (select count(*) from evidence) + (select count(*) from assertion)").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	_, err = bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-local-notes-file",
		Evidence: []wire.EvidenceInput{{Ref: "notes", SourceKind: "document", SourceURI: "file:///Users/bob/notes/callbacks.md", SourceRevision: "2026-10-21"}},
		Links:    []wire.LinkInput{{Claim: refs["drops"].ID, Evidence: []string{"notes"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "local source") {
		t.Errorf("citing the local file: err = %v, want it refused as a local source", err)
	}
	if after := count(); after != before {
		t.Errorf("the refused call wrote %d rows", after-before)
	}

	// What the notes rest on: the work item they name, which corroborates the
	// document's unmeasured deduction, and what bob agreed to state himself.
	said := time.Date(2026, 10, 21, 9, 30, 0, 0, time.UTC)
	notes, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-local-notes-callbacks", Rationale: "sources named in bob's notes",
		Evidence: []wire.EvidenceInput{
			{Ref: "wi419", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/419", SourceRevision: "3"},
			{Ref: "said", SourceKind: "statement", Excerpt: "The gateway dropped callbacks during the 2.3 load test in staging.", ObservedAt: &said},
		},
		Links: []wire.LinkInput{{Claim: refs["drops"].ID, Evidence: []string{"wi419"}}},
		Assertions: []wire.AssertionInput{{Kind: "observation", Subject: refs["gateway"].ID, Evidence: []string{"said"},
			BodyMarkdown: "The callback gateway dropped callbacks during the 2.3 load test in staging", Scope: &wire.Scope{Environment: "staging"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes.Linked) != 1 || notes.Linked[0].RootsAdded != 1 || notes.Linked[0].Support.Roots != 2 || !notes.Linked[0].Support.MeetsThreshold {
		t.Errorf("linked = %+v, want the deduction gaining the work item's root and meeting its threshold", notes.Linked)
	}
	if w := written(notes)["assertion-1"]; w.Support == nil || w.Support.Roots != 1 || w.Support.MeetsThreshold {
		t.Errorf("statement observation = %+v, want one statement root below threshold", w.Support)
	}
	rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: written(notes)["assertion-1"].ID})
	if err != nil || len(rec.Citations) != 1 || rec.Citations[0].SourceKind != "statement" || rec.Citations[0].SourceURI != "person://bob" {
		t.Errorf("statement observation read back = %+v, %v; want bob's statement", rec.Citations, err)
	}
}

// searchIDs returns the ids search finds in demo, best first.
func searchIDs(t *testing.T, c interface {
	Search(context.Context, wire.SearchInput) (wire.SearchOutput, error)
}, query string) []string {
	t.Helper()
	out, err := c.Search(context.Background(), wire.SearchInput{Query: query, Area: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(out.Results))
	for i, r := range out.Results {
		ids[i] = r.Record.ID
	}
	return ids
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
