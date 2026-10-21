package domain

import (
	"errors"
	"math"
	"testing"
)

func rootOf(kind SourceKind) SupportEvent {
	e := sev(SupportRootAdded)
	e.SourceKind = kind
	return e
}

// Every source kind has a root weight in the service default; observed
// sources count in full and a person's statement counts less.
func TestDefaultRootWeights(t *testing.T) {
	cfg := DefaultSupportConfig()
	for _, k := range SourceKinds() {
		w, ok := cfg.RootWeightBySourceKind[k]
		if !ok {
			t.Errorf("no default root weight for %q", k)
			continue
		}
		if k != SourceStatement && w != 1 {
			t.Errorf("default root weight of %q = %v, want 1 (observed sources keep the service-1 weight)", k, w)
		}
	}
	if s := cfg.RootWeightBySourceKind[SourceStatement]; s >= cfg.RootWeightBySourceKind[SourceCommand] || s <= 0 {
		t.Errorf("statement weight %v should be above zero and below an observed source's", s)
	}
	if got := cfg.RootWeight(""); got != 1 {
		t.Errorf("a root without a recorded kind weighs %v, want 1", got)
	}
}

func TestSupportConfigValidateRootWeights(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SupportConfig)
		want   error
	}{
		{"default", func(*SupportConfig) {}, nil},
		{"zero is allowed", func(c *SupportConfig) { c.RootWeightBySourceKind[SourceStatement] = 0 }, nil},
		{"missing kind", func(c *SupportConfig) { delete(c.RootWeightBySourceKind, SourceCommand) }, ErrInvalidSupportConfig},
		{"unknown kind", func(c *SupportConfig) { c.RootWeightBySourceKind["email"] = 1 }, ErrInvalidSupportConfig},
		{"negative", func(c *SupportConfig) { c.RootWeightBySourceKind[SourceStatement] = -0.5 }, ErrInvalidSupportConfig},
		{"one root never counts as more than one", func(c *SupportConfig) { c.RootWeightBySourceKind[SourceCommand] = 1.5 }, ErrInvalidSupportConfig},
		{"nan", func(c *SupportConfig) { c.RootWeightBySourceKind[SourceTicket] = math.NaN() }, ErrInvalidSupportConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultSupportConfig().clone()
			tt.mutate(&cfg)
			if err := cfg.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

// A root_added event earns the event weight times its root's weight.
func TestSupportScoreWeighsRootsBySourceKind(t *testing.T) {
	cfg := DefaultSupportConfig()
	w := cfg.EventWeights[SupportRootAdded]
	s := cfg.RootWeightBySourceKind[SourceStatement]
	tests := []struct {
		name   string
		events []SupportEvent
		want   float64
	}{
		{"one command run", []SupportEvent{rootOf(SourceCommand)}, w},
		{"two command runs", []SupportEvent{rootOf(SourceCommand), rootOf(SourceCommand)}, 2 * w},
		{"one statement", []SupportEvent{rootOf(SourceStatement)}, s * w},
		{"command and statement", []SupportEvent{rootOf(SourceCommand), rootOf(SourceStatement)}, w + s*w},
		{"root stored before kinds were recorded", []SupportEvent{rootOf("")}, w},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SupportScore(tt.events, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-roundScore(tt.want)) > 1e-9 {
				t.Errorf("SupportScore() = %v, want %v", got, tt.want)
			}
		})
	}
	// A statement alone does not meet an observation's threshold by default.
	if one, _ := SupportScore([]SupportEvent{rootOf(SourceStatement)}, cfg); cfg.MeetsThreshold(KindObservation, one) {
		t.Errorf("one statement (%v) meets the observation threshold; it should count less than an observed source", one)
	}
}

func TestRootsBySourceKind(t *testing.T) {
	cfg := DefaultSupportConfig()
	events := []SupportEvent{rootOf(SourceStatement), rootOf(SourceCommand), sev(SupportVerified), rootOf(SourceCommand), rootOf("")}
	got := RootsBySourceKind(events, cfg)
	want := []RootKindSupport{
		{Kind: SourceCommand, Roots: 2, Weight: 1},
		{Kind: SourceStatement, Roots: 1, Weight: cfg.RootWeightBySourceKind[SourceStatement]},
		{Kind: "", Roots: 1, Weight: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("RootsBySourceKind() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RootsBySourceKind()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if RootsBySourceKind(nil, cfg) != nil {
		t.Errorf("no events: want nil")
	}
}
