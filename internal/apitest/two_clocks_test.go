//go:build integration

package apitest

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/client"
	"github.com/rztaylor/froodbase/internal/wire"
)

// TestTwoClocks drives the slice 2 reads through internal/client exactly as
// froodbase-mcp does (development guide, "What is true now, what was true
// then"): records contributed with and without validity dates and scope,
// then searched in each mode, read with get_as_of before, during and after a
// record's validity, and flagged stale once unchecked for longer than the
// area's freshness policy allows. The pitch's 2.3 failure is superseded by
// setting its state, as slice 4's correction will.
func TestTwoClocks(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	bob, carol, dave := env.Client("bob"), env.Client("carol"), env.Client("dave")
	now := time.Now().UTC().Truncate(time.Second)
	days := func(n int) *time.Time { d := now.AddDate(0, 0, n); return &d }
	incident, deployed := days(-40), days(-30)

	const ticket = "https://dev.azure.com/org/p/_workitems/edit/418"
	out, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-two-clocks", Rationale: "INC-12 timeline",
		Entities: []wire.EntityInput{{Ref: "svc", Type: "service", Names: []wire.Name{{Name: "Callback Service"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "repro", SourceURI: ticket, SourceRevision: "3"},
			{Ref: "deploy", SourceURI: ticket, SourceRevision: "9"},
			{Ref: "old", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/77", SourceRevision: "1"},
			{Ref: "doc", SourceURI: "https://github.com/org/repo/blob/main/docs/callbacks.md", SourceRevision: "main@2026-10-01"},
		},
		Assertions: []wire.AssertionInput{
			{Ref: "failure", Kind: "observation", Subject: "svc", Evidence: []string{"repro"}, ValidFrom: incident,
				BodyMarkdown: "Callback failure reproduced in production on version 2.3", Scope: &wire.Scope{Environment: "prod", VersionMin: "2.3", VersionMax: "2.3"}},
			{Ref: "deployment", Kind: "observation", Subject: "svc", Evidence: []string{"deploy"}, ValidFrom: deployed,
				BodyMarkdown: "Version 2.4 deployed to production and the callback remediation verified", Scope: &wire.Scope{Environment: "prod", VersionMin: "2.4", VersionMax: "2.4"}},
			{Ref: "import", Kind: "inference", Subject: "svc", Evidence: []string{"old"}, ValidFrom: incident, ValidTo: deployed,
				BodyMarkdown: "The earlier investigation concluded the callbacks failed on 2.3 until the fix", Scope: &wire.Scope{Environment: "prod", VersionMin: "2.3", VersionMax: "2.3"}},
			{Ref: "undated", Kind: "observation", Subject: "svc", Evidence: []string{"doc"},
				BodyMarkdown: "Callback retries are documented as three attempts"},
		},
	})
	if err != nil {
		t.Fatalf("contribute: %v", err)
	}
	w := written(out)
	ids := map[string]string{}
	for _, ref := range []string{"failure", "deployment", "import", "undated"} {
		ids[w[ref].ID] = ref
	}
	if _, err := env.DB.Admin.Pgx().Exec(ctx, "update assertion set lifecycle_state = 'superseded' where id = $1", w["failure"].ID); err != nil {
		t.Fatal(err)
	}

	search := func(c *client.Client, in wire.SearchInput) map[string]wire.Record {
		t.Helper()
		in.Query = "callback"
		res, err := c.Search(ctx, in)
		if err != nil {
			t.Fatalf("search %+v: %v", in, err)
		}
		got := map[string]wire.Record{}
		for _, r := range res.Results {
			if ref, ok := ids[r.Record.ID]; ok {
				got[ref] = r.Record
			}
		}
		return got
	}
	refs := func(m map[string]wire.Record) []string {
		out := []string{}
		for ref := range m {
			out = append(out, ref)
		}
		sort.Strings(out)
		return out
	}
	statuses := func(m map[string]wire.Record) map[string]string {
		out := map[string]string{}
		for ref, r := range m {
			out[ref] = r.Status
		}
		return out
	}

	// 1. What is true now: the superseded failure and the ended
	// investigation are left out; the undated record is current but marked
	// validity unknown, with no dates taken from its recording and no
	// verification.
	cur := search(bob, wire.SearchInput{})
	if got, want := refs(cur), []string{"deployment", "undated"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("current = %v, want %v", got, want)
	}
	if u := cur["undated"]; !u.ValidityUnknown || u.ValidFrom != nil || u.ValidTo != nil || u.VerifiedAt != nil || u.Status != wire.StatusCurrent || u.Stale {
		t.Errorf("undated record = %+v, want current, validity unknown, no dates and never verified", u)
	}
	if d := cur["deployment"]; d.ValidityUnknown || d.ValidFrom == nil || !d.ValidFrom.Equal(*deployed) || d.Status != wire.StatusCurrent {
		t.Errorf("deployment = %+v, want current from %s", d, deployed)
	}

	// 2. What was true then, by validity time: the investigation recorded
	// today about the incident is included, the superseded failure is kept
	// and labelled, and the deployment had not happened.
	then := search(bob, wire.SearchInput{Mode: wire.ModeAsOf, AsOf: days(-35)})
	if got, want := refs(then), []string{"failure", "import", "undated"}; !reflect.DeepEqual(got, want) {
		t.Errorf("as_of 35 days ago = %v, want %v", got, want)
	}
	if got, want := statuses(then), map[string]string{"failure": wire.StatusHistorical, "import": wire.StatusHistorical, "undated": wire.StatusCurrent}; !reflect.DeepEqual(got, want) {
		t.Errorf("as_of statuses = %v, want %v", got, want)
	}

	// 3. Everything, each labelled relative to now.
	all := search(bob, wire.SearchInput{Mode: wire.ModeHistorical})
	want := map[string]string{"failure": wire.StatusHistorical, "import": wire.StatusHistorical, "deployment": wire.StatusCurrent, "undated": wire.StatusCurrent}
	if got := statuses(all); !reflect.DeepEqual(got, want) {
		t.Errorf("historical statuses = %v, want %v", got, want)
	}
	if all["failure"].LifecycleState != "superseded" {
		t.Errorf("failure state = %q, want superseded", all["failure"].LifecycleState)
	}

	// 4. Scope: production on 2.3 leaves out the 2.4 deployment; the
	// undated record has no scope, so it is not ruled out.
	scoped := search(bob, wire.SearchInput{Mode: wire.ModeHistorical, Scope: &wire.Scope{Environment: "PROD", VersionMin: "2.3", VersionMax: "2.3"}})
	if got, want := refs(scoped), []string{"failure", "import", "undated"}; !reflect.DeepEqual(got, want) {
		t.Errorf("historical on prod 2.3 = %v, want %v", got, want)
	}

	// 5. get_as_of reads one record by validity time.
	imp := w["import"].ID
	for _, c := range []struct {
		name  string
		at    *time.Time
		valid bool
		says  string
	}{
		{"before its validity", days(-50), false, "valid from"},
		{"during it, before it was recorded", days(-35), true, ""},
		{"after it ended", days(-10), false, "ended"},
	} {
		got, err := bob.GetAsOf(ctx, wire.GetAsOfInput{ID: imp, AsOf: c.at})
		if err != nil {
			t.Fatalf("get_as_of %s: %v", c.name, err)
		}
		if got.Valid != c.valid || (c.valid && (got.Record == nil || got.Record.ID != imp || got.Record.Kind != "inference")) ||
			(!c.valid && (got.Record != nil || !strings.Contains(got.Reason, c.says))) {
			t.Errorf("get_as_of %s = %+v, want valid %v (reason mentioning %q)", c.name, got, c.valid, c.says)
		}
	}
	if got, err := bob.GetAsOf(ctx, wire.GetAsOfInput{ID: w["undated"].ID, AsOf: days(-1000)}); err != nil || !got.Valid || !got.Record.ValidityUnknown {
		t.Errorf("get_as_of undated = %+v, %v; want valid and marked validity unknown", got, err)
	}
	if got, err := bob.GetAsOf(ctx, wire.GetAsOfInput{ID: w["svc"].ID, AsOf: days(-35)}); err != nil || !got.Valid || got.Record.Entity == nil || len(got.Record.Entity.Names) != 1 {
		t.Errorf("get_as_of entity = %+v, %v; want valid with its one name", got, err)
	}
	if _, err := bob.GetAsOf(ctx, wire.GetAsOfInput{ID: imp}); err == nil || !strings.Contains(err.Error(), "as_of is required") {
		t.Errorf("get_as_of without a time: err = %v", err)
	}

	// 6. Stale: once the deployment has gone unchecked for longer than the
	// default 90 days it is flagged, and still returned in current mode.
	if _, err := env.DB.Admin.Pgx().Exec(ctx, "update assertion set verified_at = $2 where id = $1", w["deployment"].ID, now.AddDate(0, 0, -100)); err != nil {
		t.Fatal(err)
	}
	cur = search(bob, wire.SearchInput{})
	if d, ok := cur["deployment"]; !ok || !d.Stale || d.Status != wire.StatusCurrent {
		t.Errorf("deployment after 100 days unchecked = %+v (found %v), want current and stale", d, ok)
	}
	if cur["undated"].Stale {
		t.Error("the undated record, recorded today, is stale")
	}
	rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: w["deployment"].ID})
	if err != nil || !rec.Stale || rec.Status != wire.StatusCurrent {
		t.Errorf("get_record deployment = %+v, %v; want current and stale", rec, err)
	}

	// 7. Malformed questions are refused with a reason; another area's
	// reviewer sees nothing in any mode.
	for _, c := range []struct {
		in   wire.SearchInput
		says string
	}{
		{wire.SearchInput{Query: "callback", Mode: wire.ModeAsOf}, "needs as_of"},
		{wire.SearchInput{Query: "callback", AsOf: days(-1)}, "set mode to as_of"},
		{wire.SearchInput{Query: "callback", Mode: "latest"}, "not current, as_of or historical"},
		{wire.SearchInput{Query: "callback", Scope: &wire.Scope{VersionMin: "2.4-beta"}}, "scope"},
	} {
		if _, err := bob.Search(ctx, c.in); err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("search %+v: err = %v, want one saying %q", c.in, err, c.says)
		}
	}
	if got := search(dave, wire.SearchInput{Mode: wire.ModeHistorical}); len(got) != 0 {
		t.Errorf("dave (other area) historical = %v, want nothing", refs(got))
	}
	if _, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-bad-scope",
		Evidence: []wire.EvidenceInput{{Ref: "e", SourceURI: ticket, SourceRevision: "10"}},
		Assertions: []wire.AssertionInput{{Kind: "observation", Subject: w["svc"].ID, Evidence: []string{"e"},
			BodyMarkdown: "Callbacks fail on the latest build", Scope: &wire.Scope{VersionMin: "latest"}}},
	}); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Errorf("contribute with an unreadable version: err = %v, want a scope refusal", err)
	}
}
