package store

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
)

var (
	tsA = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	tsB = time.Date(2026, 9, 9, 12, 30, 0, 0, time.UTC)
)

func ptr(t time.Time) *time.Time { return &t }

// A revision snapshot must give back the record it was taken from (TR-35).
func TestEntitySnapshotRoundTrip(t *testing.T) {
	e, err := domain.NewEntity("e1", "a1", domain.EntityService, domain.EntityName{Name: "Callback Service", Scope: domain.Scope{Environment: "prod"}, ValidFrom: ptr(tsA)})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AddName(domain.EntityName{Name: "callback-svc", ValidTo: ptr(tsB)}); err != nil {
		t.Fatal(err)
	}
	in := EntityRecord{Entity: e, LifecycleState: LifecycleActive, Revision: 3}
	b, err := entitySnapshotBytes(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := EntityFromSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if out.LifecycleState != in.LifecycleState || out.Revision != in.Revision {
		t.Errorf("state/revision = %q/%d, want %q/%d", out.LifecycleState, out.Revision, in.LifecycleState, in.Revision)
	}
	if out.Entity.ID() != e.ID() || out.Entity.AreaID != e.AreaID || out.Entity.Type != e.Type {
		t.Errorf("identity = %+v, want %+v", out.Entity, e)
	}
	if !reflect.DeepEqual(out.Entity.Names, e.Names) {
		t.Errorf("names = %+v, want %+v", out.Entity.Names, e.Names)
	}
	// The stored shape names the fields the architecture names.
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "area_id", "entity_type", "names", "lifecycle_state", "revision"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("snapshot lacks %q: %s", k, b)
		}
	}
}

func TestEntitySnapshotRejectsNoNames(t *testing.T) {
	if _, err := EntityFromSnapshot([]byte(`{"id":"e1","area_id":"a1","entity_type":"service","names":[],"lifecycle_state":"active","revision":1}`)); err == nil {
		t.Error("EntityFromSnapshot accepted an entity without names")
	}
	if _, err := EntityFromSnapshot([]byte(`not json`)); err == nil {
		t.Error("EntityFromSnapshot accepted malformed JSON")
	}
}

func TestAssertionSnapshotRoundTrip(t *testing.T) {
	a, err := domain.NewObservation("as1", "a1", "e1", "The API returned 503s.", domain.Scope{Environment: "prod", VersionMin: "2.3"}, tsA)
	if err != nil {
		t.Fatal(err)
	}
	a.ValidFrom = ptr(tsA)
	a.VerifiedAt = ptr(tsB)
	in := AssertionRecord{Assertion: a, LifecycleState: LifecycleActive, Revision: 2, SupportScore: 2, SupportConfigVersion: "service-1"}
	b, err := assertionSnapshotBytes(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := AssertionFromSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Assertion, a) {
		t.Errorf("assertion = %+v, want %+v", out.Assertion, a)
	}
	if out.LifecycleState != LifecycleActive || out.Revision != 2 {
		t.Errorf("state/revision = %q/%d", out.LifecycleState, out.Revision)
	}
	// Support is a projection of the support events, not content: it is
	// never part of a content snapshot (TR-29).
	if out.SupportScore != 0 || out.SupportConfigVersion != "" {
		t.Errorf("snapshot carried support %v/%q", out.SupportScore, out.SupportConfigVersion)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["support_score"]; ok {
		t.Errorf("snapshot stores a support score: %s", b)
	}
	if _, ok := raw["valid_to"]; ok {
		t.Errorf("unset valid_to is present: %s", b)
	}
}

func TestAssertionSnapshotRejectsInvalidRecord(t *testing.T) {
	if _, err := AssertionFromSnapshot([]byte(`{"id":"as1","area_id":"a1","subject_entity_id":"e1","epistemic_kind":"guess","body_markdown":"x","scope":{},"recorded_at":"2026-09-02T10:00:00Z","lifecycle_state":"active","revision":1}`)); err == nil {
		t.Error("AssertionFromSnapshot accepted an unknown epistemic kind")
	}
}

func TestEvidenceSnapshotRoundTrip(t *testing.T) {
	in := domain.Evidence{
		ID: "ev1", AreaID: "a1", SourceKind: domain.SourceRepository, SourceURI: "https://github.com/rztaylor/Froodbase/blob/main/README.md",
		SourceRevision: "1397355", SourceLocator: "L1-L3", Excerpt: "Froodbase is...", AccessClass: domain.AccessArea, RootEvidenceID: "ev0",
		ObservedAt: ptr(tsA), RecordedAt: tsB,
	}
	b, err := evidenceSnapshotBytes(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := EvidenceFromSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("evidence = %+v, want %+v", out, in)
	}
	// Minimal evidence: no locator, excerpt, root or observation time.
	minimal := domain.Evidence{ID: "ev2", AreaID: "a1", SourceKind: domain.SourceTicket, SourceURI: "https://dev.azure.com/org/p/_workitems/edit/1", SourceRevision: "4", AccessClass: domain.AccessRestricted, RecordedAt: tsA}
	b, err = evidenceSnapshotBytes(minimal)
	if err != nil {
		t.Fatal(err)
	}
	out, err = EvidenceFromSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, minimal) {
		t.Errorf("minimal evidence = %+v, want %+v", out, minimal)
	}
	if _, err := EvidenceFromSnapshot([]byte(`{"id":"ev3","area_id":"a1","source_kind":"document","source_uri":"/tmp/x","source_revision":"1","access_class":"area","recorded_at":"2026-09-02T10:00:00Z"}`)); err == nil {
		t.Error("EvidenceFromSnapshot accepted a local source")
	}
}

func TestScopeBytes(t *testing.T) {
	if got := string(scopeBytes(domain.Scope{})); got != "{}" {
		t.Errorf("unspecified scope = %s, want {}", got)
	}
	s := domain.Scope{Environment: "prod", Region: "uksouth", VersionMin: "2.3", VersionMax: "2.4", Cluster: "c1", Tenant: "t1"}
	got, err := scopeFromBytes(scopeBytes(s))
	if err != nil || got != s {
		t.Errorf("scope round trip = %+v, %v; want %+v", got, err, s)
	}
	if got, err := scopeFromBytes(nil); err != nil || !got.IsUnspecified() {
		t.Errorf("scopeFromBytes(nil) = %+v, %v", got, err)
	}
	if _, err := scopeFromBytes([]byte(`[1]`)); err == nil {
		t.Error("scopeFromBytes accepted a non-object")
	}
}

// The support configuration row round-trips the domain configuration, and
// the service default the migration seeds is the domain default.
func TestSupportConfigRowRoundTrip(t *testing.T) {
	cfg := domain.DefaultSupportConfig()
	weights, roots, thresholds, allowed, err := supportConfigToRow(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := supportConfigFromRow(cfg.Version, weights, roots, thresholds, allowed, cfg.UseWithoutChallengeCap)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("config = %+v, want %+v", got, cfg)
	}
	bad := cfg
	bad.EventWeights = map[domain.SupportEventType]float64{"retrieved": 1}
	if _, _, _, _, err := supportConfigToRow(bad); err == nil {
		t.Error("supportConfigToRow accepted an invalid configuration")
	}
	if _, err := supportConfigFromRow("v", []byte(`{}`), roots, thresholds, allowed, 0); err == nil {
		t.Error("supportConfigFromRow accepted missing weights")
	}
	if _, err := supportConfigFromRow("v", []byte(`nope`), roots, thresholds, allowed, 0); err == nil {
		t.Error("supportConfigFromRow accepted malformed weights")
	}
	if _, err := supportConfigFromRow(cfg.Version, weights, []byte(`{"repository": 1}`), thresholds, allowed, cfg.UseWithoutChallengeCap); err == nil {
		t.Error("supportConfigFromRow accepted root weights missing source kinds")
	}
}

func TestNewID(t *testing.T) {
	a, b := NewID(), NewID()
	if a == b {
		t.Fatalf("two ids are equal: %s", a)
	}
	if len(a) != 36 || a[14] != '4' {
		t.Errorf("NewID() = %q, want a version 4 UUID", a)
	}
}
