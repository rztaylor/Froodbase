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

// TestCommandObservationAndStatement is the FRB-50 scenario, driven through
// internal/client as froodbase-mcp drives the API: an agent records a fact
// it found by running az against a web app, and bob's own statement about
// it; someone else reads both back with the citations and the support by
// source kind. Running the command again later is a second root. A secret
// in the output, someone else's statement, and a statement attributed to a
// different person are refused and write nothing.
func TestCommandObservationAndStatement(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	bob, carol := env.Client("bob"), env.Client("carol")

	const app = "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool"
	const az = "az webapp config show --ids /subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool --query http20Enabled"
	ran := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	said := ran.Add(15 * time.Minute)
	out, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "cmd-1", Rationale: "checked during INC-9",
		Entities: []wire.EntityInput{{Ref: "app", Type: "service", Names: []wire.Name{{Name: "Import Tool web app"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "run", SourceKind: "command", SourceURI: app, Locator: az, Excerpt: "true", AccessClass: "area", ObservedAt: &ran},
			{Ref: "bob-said", SourceKind: "statement", Excerpt: "We turned HTTP/2 on for the import tool in September.", AccessClass: "area", ObservedAt: &said},
		},
		Assertions: []wire.AssertionInput{
			{Ref: "http2", Kind: "observation", Subject: "app", BodyMarkdown: "HTTP/2 is enabled on the import tool web app", Evidence: []string{"run"}},
			{Ref: "since", Kind: "observation", Subject: "app", BodyMarkdown: "HTTP/2 was turned on for the import tool in September 2026", Evidence: []string{"bob-said"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := written(out)
	http2, since := refs["http2"], refs["since"]
	if s := http2.Support; s == nil || s.Roots != 1 || s.Score != 1 || !s.MeetsThreshold ||
		!reflect.DeepEqual(s.RootsBySourceKind, []wire.RootWeight{{SourceKind: "command", Roots: 1, Weight: 1}}) {
		t.Fatalf("command observation support = %+v, want one command root weighing 1", s)
	}
	if s := since.Support; s == nil || s.Roots != 1 || s.Score != 0.5 || s.MeetsThreshold ||
		!reflect.DeepEqual(s.RootsBySourceKind, []wire.RootWeight{{SourceKind: "statement", Roots: 1, Weight: 0.5}}) {
		t.Fatalf("statement support = %+v, want one statement root weighing 0.5, below threshold", s)
	}

	// Carol reads both back with their citations.
	rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: http2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Citations) != 1 {
		t.Fatalf("citations = %+v", rec.Citations)
	}
	if c := rec.Citations[0]; c.SourceKind != "command" || c.SourceURI != app || c.Locator != az || c.Excerpt != "true" ||
		c.ObservedAt == nil || !c.ObservedAt.Equal(ran) || c.SourceRevision != "2026-10-21T09:00:00Z" {
		t.Errorf("command citation = %+v", c)
	}
	if !reflect.DeepEqual(rec.Support.RootsBySourceKind, http2.Support.RootsBySourceKind) {
		t.Errorf("get_record support = %+v, want the contribute result's %+v", rec.Support, http2.Support)
	}
	rec, err = carol.GetRecord(ctx, wire.GetRecordInput{ID: since.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c := rec.Citations[0]; c.SourceKind != "statement" || c.SourceURI != "person://bob" || !strings.Contains(c.Excerpt, "September") || c.ObservedAt == nil {
		t.Errorf("statement citation = %+v", c)
	}
	if rec.Support.Score != 0.5 || rec.Support.MeetsThreshold || rec.Support.RootsBySourceKind[0].Weight != 0.5 {
		t.Errorf("statement record support = %+v", rec.Support)
	}

	// The same command run again the next day is a second root.
	next := ran.Add(24 * time.Hour)
	again, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "cmd-2", Rationale: "re-checked",
		Evidence:   []wire.EvidenceInput{{Ref: "run2", SourceKind: "command", SourceURI: app, Locator: az, Excerpt: "true", AccessClass: "area", ObservedAt: &next}},
		Assertions: []wire.AssertionInput{{Ref: "still", Kind: "observation", Subject: refs["app"].ID, BodyMarkdown: "HTTP/2 is still enabled on the import tool web app", Evidence: []string{refs["run"].ID, "run2"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s := written(again)["still"].Support; s.Roots != 2 || s.Score != 2 || s.RootsBySourceKind[0] != (wire.RootWeight{SourceKind: "command", Roots: 2, Weight: 1}) {
		t.Errorf("two runs: support %+v, want two command roots", s)
	}

	// Refusals write nothing.
	count := func() int {
		var n int
		if err := env.DB.Admin.Pgx().QueryRow(ctx, "select (select count(*) from evidence) + (select count(*) from assertion)").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	for name, tc := range map[string]struct {
		ev   wire.EvidenceInput
		want string
	}{
		"secret in the output": {
			wire.EvidenceInput{Ref: "e", SourceKind: "command", SourceURI: "sql://sql-import.database.windows.net/importdb", Locator: "az sql db show-connection-string -s sql-import -n importdb -c ado.net",
				Excerpt: "Server=tcp:sql-import.database.windows.net;Database=importdb;User ID=app;Password=Sup3rS3cret!;", AccessClass: "area", ObservedAt: &ran},
			"looks like it holds a secret (a password in a connection string)",
		},
		"bearer token in the command": {
			wire.EvidenceInput{Ref: "e", SourceKind: "command", SourceURI: "host://api.contoso.net", Locator: "curl -H 'Authorization: Bearer abcdefghijklmnop0123456789' https://api.contoso.net/health",
				AccessClass: "area", ObservedAt: &ran},
			"bearer token",
		},
		"someone else's statement": {
			wire.EvidenceInput{Ref: "e", SourceKind: "statement", SourceURI: "person://alice", Excerpt: "Alice said so.", AccessClass: "area", ObservedAt: &said},
			"only record their own statements",
		},
		"statement without a time": {
			wire.EvidenceInput{Ref: "e", SourceKind: "statement", Excerpt: "I think so.", AccessClass: "area"},
			"observed_at is required",
		},
	} {
		_, err := bob.Contribute(ctx, wire.ContributeInput{
			Area: "demo", IdempotencyKey: "refused-" + name, Evidence: []wire.EvidenceInput{tc.ev},
			Assertions: []wire.AssertionInput{{Ref: "o", Kind: "observation", Subject: refs["app"].ID, BodyMarkdown: "Refused", Evidence: []string{"e"}}},
		})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if err != nil && (strings.Contains(err.Error(), "Sup3rS3cret") || strings.Contains(err.Error(), "abcdefghijklmnop")) {
			t.Errorf("%s: the refusal repeats the secret: %v", name, err)
		}
	}
	if after := count(); after != before {
		t.Errorf("refused contributions wrote %d rows", after-before)
	}
}
