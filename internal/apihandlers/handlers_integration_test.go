//go:build integration

package apihandlers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/contribute"
	"github.com/rztaylor/froodbase/internal/retrieval"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/store/storetest"
	"github.com/rztaylor/froodbase/internal/wire"
)

func caller(user string, roles map[string][]authz.Role) *authz.Context {
	return &authz.Context{UserID: user, DisplayName: user, Grants: roles}
}

func input(key string) wire.ContributeInput {
	return wire.ContributeInput{
		Area: "demo", IdempotencyKey: key, Rationale: "seen during INC-1",
		Entities: []wire.EntityInput{{Ref: "tool", Type: "service", Names: []wire.Name{{Name: "Import Tool"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "ticket", SourceKind: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/418", SourceRevision: "2026-10-20T10:00:00Z", AccessClass: "area"},
			{Ref: "quote", SourceKind: "document", SourceURI: "https://contoso.sharepoint.com/sites/eng/runbook.docx", SourceRevision: "v7", Locator: "section 3", AccessClass: "area", Root: "ticket"},
		},
		Observations: []wire.ObservationInput{{
			Ref: "obs", Subject: "tool", BodyMarkdown: "Callback failure reproduced on version 2.3", Scope: &wire.Scope{Environment: "prod", VersionMin: "2.3", VersionMax: "2.3"},
			Evidence: []string{"ticket", "quote"},
		}},
	}
}

func TestContributeReadBackAndHistory(t *testing.T) {
	ctx := context.Background()
	pool := storetest.Open(t).App // the service role: row-level security applies
	areas := NewAreas(pool)
	if created, err := areas.Ensure(ctx, []string{"demo", "other"}); err != nil || len(created) != 2 {
		t.Fatalf("Ensure = %v, %v", created, err)
	}
	if created, err := areas.Ensure(ctx, []string{"demo"}); err != nil || len(created) != 0 {
		t.Fatalf("second Ensure = %v, %v", created, err)
	}
	h := New(pool, areas, "test/0")
	bob := caller("bob", map[string][]authz.Role{"demo": {authz.RoleContributor}})
	withBob := authz.WithContext(ctx, bob)

	out, err := h.Contribute(withBob, input("k1"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Replayed || out.Area != "demo" || len(out.Records) != 4 {
		t.Fatalf("contribute = %+v", out)
	}
	var obs, tool wire.Written
	for _, r := range out.Records {
		switch r.Ref {
		case "obs":
			obs = r
		case "tool":
			tool = r
		}
	}
	if obs.Kind != "observation" || obs.LifecycleState != "active" || obs.Support == nil || obs.Support.Roots != 1 || obs.Support.Score != 1 || !obs.Support.MeetsThreshold {
		t.Fatalf("observation written = %+v support %+v", obs, obs.Support)
	}

	// Same key, same request: replayed, nothing new.
	again, err := h.Contribute(withBob, input("k1"))
	if err != nil {
		t.Fatal(err)
	}
	if !again.Replayed || again.ContributionID != out.ContributionID || len(again.Records) != 4 || again.Records[0].ID != out.Records[0].ID {
		t.Fatalf("replay = %+v", again)
	}
	// Same key, different request: refused.
	changed := input("k1")
	changed.Rationale = "different"
	if _, err := h.Contribute(withBob, changed); !errors.Is(err, store.ErrIdempotencyMismatch) {
		t.Fatalf("mismatched replay err = %v", err)
	}
	// New key, same content: new records (the source is already held, so cite it).
	fresh := input("k2")
	fresh.Evidence = nil
	fresh.Observations[0].Evidence = []string{evidenceID(out, "ticket")}
	fresh.Observations[0].BodyMarkdown = "Seen again"
	more, err := h.Contribute(withBob, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if more.Replayed || more.ContributionID == out.ContributionID {
		t.Fatalf("new key = %+v", more)
	}

	// Read back.
	rec, err := h.GetRecord(withBob, wire.GetRecordInput{ID: obs.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != "observation" || rec.LifecycleState != "active" || rec.Support == nil || rec.Support.Roots != 1 || rec.Support.Events != 1 ||
		rec.Scope == nil || rec.Scope.Environment != "prod" || !rec.ValidityUnknown || len(rec.Citations) != 2 || rec.Subject != tool.ID || rec.Area != "demo" {
		t.Fatalf("get_record = %+v", rec)
	}
	ent, err := h.GetRecord(withBob, wire.GetRecordInput{ID: tool.ID})
	if err != nil || ent.Entity == nil || ent.Entity.Type != "service" || ent.Entity.Names[0].Name != "Import Tool" {
		t.Fatalf("get_record entity = %+v, %v", ent, err)
	}
	ev, err := h.GetRecord(withBob, wire.GetRecordInput{ID: evidenceID(out, "quote")})
	if err != nil || ev.Evidence == nil || ev.Evidence.RootEvidenceID != evidenceID(out, "ticket") {
		t.Fatalf("get_record evidence = %+v, %v", ev, err)
	}
	hist, err := h.GetHistory(withBob, wire.GetHistoryInput{ID: obs.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Revisions) != 1 || hist.Revisions[0].ActorUserID != "bob" || hist.Revisions[0].Rationale != "seen during INC-1" || len(hist.SupportEvents) != 1 || hist.SupportEvents[0].Type != "root_added" {
		t.Fatalf("get_history = %+v", hist)
	}

	// A reader cannot contribute and the refusal is audited; a stranger sees nothing.
	carol := caller("carol", map[string][]authz.Role{"demo": {authz.RoleReader}})
	_, err = h.Contribute(authz.WithContext(ctx, carol), input("k3"))
	if !errors.Is(err, contribute.ErrNotContributor) || !strings.Contains(err.Error(), "reader") {
		t.Fatalf("reader contribute err = %v", err)
	}
	demoID, _ := areas.AreaID(ctx, "demo")
	events, err := pool.AuditEventsForArea(ctx, store.Caller{UserID: "carol", AreaIDs: []string{string(demoID)}}, demoID)
	if err != nil {
		t.Fatal(err)
	}
	refused := 0
	for _, e := range events {
		if e.EventType == contribute.AuditContributionRefused && e.ActorUserID == "carol" {
			refused++
		}
	}
	if refused != 1 {
		t.Fatalf("refusal audit events = %d, want 1", refused)
	}
	dave := caller("dave", map[string][]authz.Role{"other": {authz.RoleReviewer}})
	if _, err := h.GetRecord(authz.WithContext(ctx, dave), wire.GetRecordInput{ID: obs.ID}); !errors.Is(err, retrieval.ErrNotFound) {
		t.Fatalf("stranger get_record err = %v", err)
	}

	// Invalid evidence rejects the whole change set.
	bad := input("k4")
	bad.Evidence[0].SourceURI = "/Users/robert/reports/incident.md"
	if _, err := h.Contribute(withBob, bad); !errors.Is(err, contribute.ErrInvalidInput) {
		t.Fatalf("local path err = %v", err)
	}
	if _, err := pool.IdempotentResult(ctx, store.Caller{UserID: "bob"}, "k4"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("rejected change set stored a key: %v", err)
	}
	unknownSubject := input("k5")
	unknownSubject.Observations[0].Subject = "00000000-0000-0000-0000-000000000000"
	if _, err := h.Contribute(withBob, unknownSubject); !errors.Is(err, contribute.ErrInvalidInput) {
		t.Fatalf("unknown subject err = %v", err)
	}
}

func TestSearchChannelsAndAuthorisation(t *testing.T) {
	ctx := context.Background()
	pool := storetest.Open(t).App
	areas := NewAreas(pool)
	if _, err := areas.Ensure(ctx, []string{"demo", "other"}); err != nil {
		t.Fatal(err)
	}
	h := New(pool, areas, "test/0")
	bob := caller("bob", map[string][]authz.Role{"demo": {authz.RoleContributor}})
	withBob := authz.WithContext(ctx, bob)
	in := input("s1")
	in.Evidence[0].AccessClass = "restricted"
	out, err := h.Contribute(withBob, in)
	if err != nil {
		t.Fatal(err)
	}
	obsID, toolID := "", ""
	for _, r := range out.Records {
		switch r.Ref {
		case "obs":
			obsID = r.ID
		case "tool":
			toolID = r.ID
		}
	}

	// Exact name.
	res, err := h.Search(withBob, wire.SearchInput{Query: "import tool"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) == 0 || res.Results[0].Record.ID != toolID || res.Results[0].Match.Channel != wire.ChannelIdentifier || res.Results[0].Match.Candidate {
		t.Fatalf("exact name search = %+v", res.Results)
	}
	// Misspelt name within the trigram threshold is a candidate.
	res, err = h.Search(withBob, wire.SearchInput{Query: "Improt Tool"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) == 0 || res.Results[0].Record.ID != toolID || !res.Results[0].Match.Candidate || res.Results[0].Match.Similarity <= 0 {
		t.Fatalf("misspelt name search = %+v", res.Results)
	}
	// Far from any name: nothing.
	res, err = h.Search(withBob, wire.SearchInput{Query: "zzzzqqqq"})
	if err != nil || len(res.Results) != 0 {
		t.Fatalf("no-match search = %+v, %v", res.Results, err)
	}
	// A word in the body finds the observation with its full fields.
	res, err = h.Search(withBob, wire.SearchInput{Query: "callback"})
	if err != nil {
		t.Fatal(err)
	}
	var found *wire.SearchResult
	for i := range res.Results {
		if res.Results[i].Record.ID == obsID {
			found = &res.Results[i]
		}
	}
	if found == nil || found.Match.Channel != wire.ChannelFullText || found.Record.Kind != "observation" || found.Record.LifecycleState != "active" ||
		found.Record.Support == nil || found.Record.Scope == nil || !found.Record.ValidityUnknown || len(found.Record.Citations) != 2 {
		t.Fatalf("body search = %+v", found)
	}
	// Restricted evidence shown with the claim is audited.
	demoID, _ := areas.AreaID(ctx, "demo")
	events, err := pool.AuditEventsForArea(ctx, store.Caller{UserID: "bob", AreaIDs: []string{string(demoID)}}, demoID)
	if err != nil {
		t.Fatal(err)
	}
	restricted := 0
	for _, e := range events {
		if e.EventType == retrieval.AuditRestrictedRead {
			restricted++
		}
	}
	if restricted == 0 {
		t.Fatal("no restricted_read audit event after showing restricted evidence")
	}
	// A caller without the area sees nothing and no count.
	dave := caller("dave", map[string][]authz.Role{"other": {authz.RoleReviewer}})
	res, err = h.Search(authz.WithContext(ctx, dave), wire.SearchInput{Query: "Import Tool"})
	if err != nil || len(res.Results) != 0 {
		t.Fatalf("stranger search = %+v, %v", res, err)
	}
	// Area filter the caller lacks: nothing.
	res, err = h.Search(withBob, wire.SearchInput{Query: "Import Tool", Area: "other"})
	if err != nil || len(res.Results) != 0 {
		t.Fatalf("other-area search = %+v, %v", res, err)
	}
	if _, err := h.Search(withBob, wire.SearchInput{Query: "  "}); !errors.Is(err, retrieval.ErrEmptyQuery) {
		t.Fatalf("empty query err = %v", err)
	}
}

func evidenceID(out wire.ContributeOutput, ref string) string {
	for _, r := range out.Records {
		if r.Ref == ref {
			return r.ID
		}
	}
	return ""
}
