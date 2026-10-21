//go:build integration

package apitest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/wire"
)

// TestPlainContribution is the FRB-51 scenario, driven through
// internal/client as froodbase-mcp drives the API: an agent turns a
// person's plain-language finding into a change set holding only what the
// request gives (no source kinds, access classes, pinned revisions or refs
// on what nothing refers to), as the froodbase-contribute skill teaches. The
// server fills in the rest, and reading back shows what it inferred. A
// later finding about the same system searches first, reuses the entity and
// cites the existing evidence by id. A retry of the plain call is replayed,
// and a link whose kind cannot be told is refused, writing nothing.
func TestPlainContribution(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	bob, carol := env.Client("bob"), env.Client("carol")

	// "Search before creating": nothing is known about the import tool yet.
	found, err := bob.Search(ctx, wire.SearchInput{Query: "Import Tool", Area: "demo"})
	if err != nil || len(found.Results) != 0 {
		t.Fatalf("search before creating = %+v, %v; want nothing", found, err)
	}

	const (
		workItem = "https://dev.azure.com/org/p/_workitems/edit/418"
		commit   = "0123456789abcdef0123456789abcdef01234567"
		code     = "https://github.com/org/import-tool/blob/" + commit + "/callback/retry.go#L40-L58"
		pod      = "k8s://aks-prod/import/deployment/import-api"
		kubectl  = "kubectl -n import get deployment import-api -o jsonpath={.spec.template.spec.containers[0].image}"
	)
	ran := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	request := wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-12-callback-retry", Rationale: "found during INC-12",
		Entities: []wire.EntityInput{{Ref: "tool", Type: "service", Names: []wire.Name{{Name: "Import Tool"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "wi", SourceURI: workItem, SourceRevision: "12"},
			{Ref: "code", SourceURI: code, Locator: "retryCallback"},
			{Ref: "run", SourceURI: pod, Locator: kubectl, Excerpt: "importtool:2.3.1", ObservedAt: &ran},
		},
		Assertions: []wire.AssertionInput{
			{Ref: "fails", Kind: "observation", Subject: "tool", BodyMarkdown: "Callbacks fail after a 30 second timeout on version 2.3", Evidence: []string{"wi"}},
			{Kind: "observation", Subject: "tool", BodyMarkdown: "Production runs import tool 2.3.1", Evidence: []string{"run"}},
			{Kind: "inference", Subject: "tool", BodyMarkdown: "The 2.3 retry change makes callbacks wait for the full timeout before retrying", DerivedFrom: []string{"fails"}, Evidence: []string{"code"}},
		},
	}
	out, err := bob.Contribute(ctx, request)
	if err != nil {
		t.Fatalf("plain contribution refused: %v", err)
	}
	refs := written(out)
	for _, r := range []string{"tool", "wi", "code", "run", "fails", "assertion-2", "assertion-3"} {
		if refs[r].ID == "" {
			t.Fatalf("result has no record for ref %q: %+v", r, out.Records)
		}
	}
	if k := refs["assertion-3"].Kind; k != "inference" {
		t.Errorf("generated-ref assertion kind = %q, want inference", k)
	}

	// Carol reads back what the server filled in.
	want := map[string]struct{ kind, revision string }{
		refs["fails"].ID:       {"ticket", "12"},
		refs["assertion-2"].ID: {"command", "2026-10-21T10:00:00Z"},
		refs["assertion-3"].ID: {"repository", commit},
	}
	for id, w := range want {
		rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if len(rec.Citations) != 1 {
			t.Fatalf("%s citations = %+v", id, rec.Citations)
		}
		if c := rec.Citations[0]; c.SourceKind != w.kind || c.SourceRevision != w.revision || c.AccessClass != "area" {
			t.Errorf("%s citation = kind %q revision %q access %q, want %s at %s, area", id, c.SourceKind, c.SourceRevision, c.AccessClass, w.kind, w.revision)
		}
	}
	rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: refs["assertion-3"].ID})
	if err != nil || !rec.Derived || len(rec.DerivedFrom) != 1 || rec.DerivedFrom[0] != refs["fails"].ID {
		t.Errorf("inference = %+v, %v; want derived from the observation", rec, err)
	}

	// Retrying the plain call is replayed, not refused as a different request.
	again, err := bob.Contribute(ctx, request)
	if err != nil || !again.Replayed || written(again)["fails"].ID != refs["fails"].ID {
		t.Fatalf("retry = %+v, %v; want the replayed result", again, err)
	}

	// A later finding: search finds the entity and the evidence, and the call
	// cites both by id instead of recording them again.
	found, err = bob.Search(ctx, wire.SearchInput{Query: "Import Tool", Area: "demo"})
	if err != nil || len(found.Results) == 0 || found.Results[0].Record.ID != refs["tool"].ID {
		t.Fatalf("search = %+v, %v; want the entity first", found, err)
	}
	later, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-12-timeout",
		Assertions: []wire.AssertionInput{{Kind: "observation", Subject: refs["tool"].ID, BodyMarkdown: "The callback timeout is 30 seconds on 2.3", Evidence: []string{refs["wi"].ID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if w := written(later)["assertion-1"]; len(later.Records) != 1 || w.Support == nil || w.Support.Roots != 1 {
		t.Errorf("later finding = %+v, want one observation citing the existing work item", later.Records)
	}

	// What the server cannot tell is still required, and nothing is written.
	count := func() int {
		var n int
		if err := env.DB.Admin.Pgx().QueryRow(ctx, "select (select count(*) from evidence) + (select count(*) from assertion)").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	for name, ev := range map[string]wire.EvidenceInput{
		"unknown link":   {SourceURI: "https://wiki.example.com/import/notes"},
		"branch link":    {SourceURI: "https://github.com/org/import-tool/blob/main/callback/retry.go"},
		"no link at all": {Excerpt: "It is the retry change"},
	} {
		_, err := bob.Contribute(ctx, wire.ContributeInput{
			Area: "demo", IdempotencyKey: "refused-" + name, Evidence: []wire.EvidenceInput{ev},
			Assertions: []wire.AssertionInput{{Kind: "observation", Subject: refs["tool"].ID, BodyMarkdown: "Refused", Evidence: []string{"evidence-1"}}},
		})
		if err == nil || !strings.Contains(err.Error(), "is required") {
			t.Errorf("%s: err = %v, want a field that is still required", name, err)
		}
	}
	if after := count(); after != before {
		t.Errorf("refused contributions wrote %d rows", after-before)
	}
}
