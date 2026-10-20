package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestParseSupportEventType(t *testing.T) {
	for _, k := range SupportEventTypes() {
		got, err := ParseSupportEventType(string(k))
		if err != nil || got != k {
			t.Errorf("ParseSupportEventType(%q) = %q, %v", k, got, err)
		}
	}
	if _, err := ParseSupportEventType("retrieved"); !errors.Is(err, ErrUnknownSupportEventType) {
		t.Errorf("ParseSupportEventType(retrieved) err = %v, want ErrUnknownSupportEventType", err)
	}
	if n := len(SupportEventTypes()); n != 9 {
		t.Errorf("SupportEventTypes() has %d types, want the nine of TR-29", n)
	}
}

func TestDefaultSupportConfigIsValid(t *testing.T) {
	cfg := DefaultSupportConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultSupportConfig().Validate() = %v", err)
	}
	if cfg.Version != DefaultSupportConfigVersion {
		t.Errorf("Version = %q, want %q", cfg.Version, DefaultSupportConfigVersion)
	}
	for _, k := range SupportEventTypes() {
		if _, ok := cfg.EventWeights[k]; !ok {
			t.Errorf("no default weight for %q", k)
		}
	}
	for _, k := range EpistemicKinds() {
		if _, ok := cfg.ThresholdByKind[k]; !ok {
			t.Errorf("no default threshold for %q", k)
		}
	}
	// Quiet use alone can never carry an inference past its threshold (PR-15).
	if cfg.UseWithoutChallengeCap >= cfg.ThresholdByKind[KindInference] {
		t.Errorf("cap %v is not below the inference threshold %v", cfg.UseWithoutChallengeCap, cfg.ThresholdByKind[KindInference])
	}
}

func TestSupportConfigValidate(t *testing.T) {
	base := DefaultSupportConfig()
	tests := []struct {
		name   string
		mutate func(*SupportConfig)
		want   error
	}{
		{"default", func(*SupportConfig) {}, nil},
		{"no version", func(c *SupportConfig) { c.Version = " " }, ErrEmptyName},
		{"missing weight", func(c *SupportConfig) { delete(c.EventWeights, SupportVerified) }, ErrInvalidSupportConfig},
		{"unknown weight", func(c *SupportConfig) { c.EventWeights["retrieved"] = 1 }, ErrInvalidSupportConfig},
		{"positive event negative", func(c *SupportConfig) { c.EventWeights[SupportRootAdded] = -1 }, ErrInvalidSupportConfig},
		{"negative event positive", func(c *SupportConfig) { c.EventWeights[SupportContradicted] = 1 }, ErrInvalidSupportConfig},
		{"negative cap", func(c *SupportConfig) { c.UseWithoutChallengeCap = -0.1 }, ErrInvalidSupportConfig},
		{"cap reaches inference threshold", func(c *SupportConfig) { c.UseWithoutChallengeCap = c.ThresholdByKind[KindInference] }, ErrInvalidSupportConfig},
		{"missing threshold", func(c *SupportConfig) { delete(c.ThresholdByKind, KindDecision) }, ErrInvalidSupportConfig},
		{"negative threshold", func(c *SupportConfig) { c.ThresholdByKind[KindObservation] = -1 }, ErrInvalidSupportConfig},
		{"nan weight", func(c *SupportConfig) { c.EventWeights[SupportVerified] = math.NaN() }, ErrInvalidSupportConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base.clone()
			tt.mutate(&cfg)
			if err := cfg.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSupportEventValidate(t *testing.T) {
	ok := SupportEvent{Type: SupportRootAdded, EvidenceRootIDs: []ID{"ev1"}, ActorUserID: "alice", ConfigVersion: "service-1", At: t1}
	tests := []struct {
		name   string
		mutate func(*SupportEvent)
		want   error
	}{
		{"actor", func(*SupportEvent) {}, nil},
		{"job", func(e *SupportEvent) { e.ActorUserID = ""; e.JobName = "review_sweep" }, nil},
		{"unknown type", func(e *SupportEvent) { e.Type = "retrieved" }, ErrUnknownSupportEventType},
		{"no actor or job", func(e *SupportEvent) { e.ActorUserID = "" }, ErrNoSupportActor},
		{"no time", func(e *SupportEvent) { e.At = time.Time{} }, ErrNoRecordedAt},
		{"no config version", func(e *SupportEvent) { e.ConfigVersion = "" }, ErrEmptyName},
		{"root_added without a root", func(e *SupportEvent) { e.EvidenceRootIDs = nil }, ErrNoEvidenceRoot},
		{"root_added with two roots", func(e *SupportEvent) { e.EvidenceRootIDs = []ID{"a", "b"} }, ErrNoEvidenceRoot},
		{"flagged without roots", func(e *SupportEvent) { e.Type = SupportFlagged; e.EvidenceRootIDs = nil }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ok
			tt.mutate(&e)
			if err := e.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func sev(typ SupportEventType) SupportEvent {
	return SupportEvent{Type: typ, EvidenceRootIDs: []ID{"r"}, ActorUserID: "alice", ConfigVersion: DefaultSupportConfigVersion, At: t1}
}

// The score is a deterministic projection of the events under the weights
// (TR-29, TR-43): roots, verifications and evidenced outcomes raise it, the
// negative events lower it, quiet use is capped and the floor is zero.
func TestSupportScore(t *testing.T) {
	cfg := DefaultSupportConfig()
	w := cfg.EventWeights
	tests := []struct {
		name   string
		events []SupportEvent
		want   float64
	}{
		{"no events", nil, 0},
		{"one root", []SupportEvent{sev(SupportRootAdded)}, w[SupportRootAdded]},
		{"two roots", []SupportEvent{sev(SupportRootAdded), sev(SupportRootAdded)}, 2 * w[SupportRootAdded]},
		{"root and verification", []SupportEvent{sev(SupportRootAdded), sev(SupportVerified)}, w[SupportRootAdded] + w[SupportVerified]},
		{"root and succeeded outcome", []SupportEvent{sev(SupportRootAdded), sev(SupportOutcomeSucceeded)}, w[SupportRootAdded] + w[SupportOutcomeSucceeded]},
		{"root then contradicted", []SupportEvent{sev(SupportRootAdded), sev(SupportContradicted)}, w[SupportRootAdded] + w[SupportContradicted]},
		{"order does not matter", []SupportEvent{sev(SupportContradicted), sev(SupportRootAdded)}, w[SupportRootAdded] + w[SupportContradicted]},
		{"never below zero", []SupportEvent{sev(SupportRootAdded), sev(SupportFlagged), sev(SupportOutcomeFailed)}, 0},
		{"quiet use is capped", []SupportEvent{
			sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge),
			sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge),
		}, cfg.UseWithoutChallengeCap},
		{"quiet use below the cap counts in full", []SupportEvent{sev(SupportUseWithoutChallenge)}, w[SupportUseWithoutChallenge]},
		{"quiet use cannot carry an inference past its threshold", []SupportEvent{
			sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge),
			sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge),
			sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge), sev(SupportUseWithoutChallenge),
		}, cfg.UseWithoutChallengeCap},
		{"every negative event", []SupportEvent{
			sev(SupportRootAdded), sev(SupportRootAdded), sev(SupportRootAdded), sev(SupportVerified), sev(SupportOutcomeSucceeded),
			sev(SupportOutcomeFailed), sev(SupportSourceChanged), sev(SupportContradicted), sev(SupportFlagged), sev(SupportSupportRetracted),
		}, 3*w[SupportRootAdded] + w[SupportVerified] + w[SupportOutcomeSucceeded] + w[SupportOutcomeFailed] + w[SupportSourceChanged] + w[SupportContradicted] + w[SupportFlagged] + w[SupportSupportRetracted]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SupportScore(tt.events, cfg)
			if err != nil {
				t.Fatalf("SupportScore() error = %v", err)
			}
			if math.Abs(got-roundScore(tt.want)) > 1e-9 {
				t.Errorf("SupportScore() = %v, want %v", got, roundScore(tt.want))
			}
			if got < 0 {
				t.Errorf("SupportScore() = %v, below zero", got)
			}
		})
	}
	// The last case must actually exceed the cap for the cap to be tested.
	if 8*w[SupportUseWithoutChallenge] <= cfg.UseWithoutChallengeCap {
		t.Errorf("test setup: 8 quiet-use events (%v) do not exceed the cap %v", 8*w[SupportUseWithoutChallenge], cfg.UseWithoutChallengeCap)
	}
	// Same events, same answer: the projection is recomputable (TR-43).
	a, _ := SupportScore(tests[len(tests)-1].events, cfg)
	b, _ := SupportScore(tests[len(tests)-1].events, cfg)
	if a != b {
		t.Errorf("SupportScore is not deterministic: %v != %v", a, b)
	}
}

func TestSupportScoreRejectsBadInput(t *testing.T) {
	cfg := DefaultSupportConfig()
	if _, err := SupportScore([]SupportEvent{sev("retrieved")}, cfg); !errors.Is(err, ErrUnknownSupportEventType) {
		t.Errorf("unknown event type: err = %v", err)
	}
	bad := cfg.clone()
	delete(bad.EventWeights, SupportVerified)
	if _, err := SupportScore([]SupportEvent{sev(SupportRootAdded)}, bad); !errors.Is(err, ErrInvalidSupportConfig) {
		t.Errorf("invalid config: err = %v", err)
	}
}

func TestSupportScoreIsRoundedToFourPlaces(t *testing.T) {
	cfg := DefaultSupportConfig()
	cfg.EventWeights[SupportRootAdded] = 0.33333
	got, err := SupportScore([]SupportEvent{sev(SupportRootAdded)}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.3333 {
		t.Errorf("SupportScore() = %v, want 0.3333", got)
	}
}

func TestMeetsThreshold(t *testing.T) {
	cfg := DefaultSupportConfig()
	cfg.ThresholdByKind[KindInference] = 2
	tests := []struct {
		score float64
		kind  EpistemicKind
		want  bool
	}{
		{1.9999, KindInference, false},
		{2, KindInference, true},
		{2.5, KindInference, true},
		{cfg.ThresholdByKind[KindObservation], KindObservation, true},
		{cfg.ThresholdByKind[KindObservation] - 0.0001, KindObservation, false},
	}
	for _, tt := range tests {
		if got := cfg.MeetsThreshold(tt.kind, tt.score); got != tt.want {
			t.Errorf("MeetsThreshold(%q, %v) = %v, want %v", tt.kind, tt.score, got, tt.want)
		}
	}
	if cfg.MeetsThreshold("lesson", 100) {
		t.Error("MeetsThreshold for a kind without a threshold = true, want false")
	}
}

// DistinctRoots is what the writer uses to append one root_added event per
// distinct root; it must agree with Corroboration.
func TestDistinctRoots(t *testing.T) {
	original := Evidence{ID: "a"}
	restated := Evidence{ID: "b", RootEvidenceID: "a"}
	other := Evidence{ID: "c"}
	external := Evidence{ID: "d", RootEvidenceID: "zz"}
	tests := []struct {
		name string
		in   []Evidence
		want []ID
	}{
		{"none", nil, nil},
		{"one", []Evidence{original}, []ID{"a"}},
		{"restatement collapses", []Evidence{original, restated}, []ID{"a"}},
		{"two originals", []Evidence{original, other}, []ID{"a", "c"}},
		{"sorted and deduplicated", []Evidence{other, restated, original, restated}, []ID{"a", "c"}},
		{"root outside the slice", []Evidence{external, other}, []ID{"c", "zz"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DistinctRoots(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("DistinctRoots() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("DistinctRoots() = %v, want %v", got, tt.want)
				}
			}
			if n := Corroboration(tt.in); n != len(got) {
				t.Errorf("Corroboration() = %d, DistinctRoots() has %d", n, len(got))
			}
		})
	}
}
