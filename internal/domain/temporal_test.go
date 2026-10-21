package domain

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTemporalValidityUnknown(t *testing.T) {
	tests := []struct {
		name     string
		from, to *time.Time
		want     bool
	}{
		{"neither date", nil, nil, true},
		{"start only", ptr(t0), nil, false},
		{"end only", nil, ptr(t2), false},
		{"both dates", ptr(t0), ptr(t2), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := Temporal{ValidFrom: tt.from, ValidTo: tt.to, RecordedAt: t1}
			if got := tm.ValidityUnknown(); got != tt.want {
				t.Errorf("ValidityUnknown() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Every constructor sets recorded_at and leaves validity null, whatever the
// recording time (TR-32).
func TestConstructorsNeverSetValidity(t *testing.T) {
	for _, kind := range EpistemicKinds() {
		for _, recorded := range []time.Time{t0, t1, t2} {
			a, err := NewAssertion(kind, "as1", "a1", "e1", "Body.", Scope{}, recorded)
			if err != nil {
				t.Fatalf("NewAssertion(%s): %v", kind, err)
			}
			if a.ValidFrom != nil || a.ValidTo != nil || a.VerifiedAt != nil || a.ReviewDueAt != nil {
				t.Errorf("NewAssertion(%s, %v) set a date other than recorded_at: %+v", kind, recorded, a.Temporal)
			}
			if !a.RecordedAt.Equal(recorded) || !a.ValidityUnknown() {
				t.Errorf("NewAssertion(%s, %v): RecordedAt %v, ValidityUnknown %v", kind, recorded, a.RecordedAt, a.ValidityUnknown())
			}
		}
	}
}

func TestTemporalValidate(t *testing.T) {
	tests := []struct {
		name string
		tm   Temporal
		want error
	}{
		{"recorded only", Temporal{RecordedAt: t1}, nil},
		{"no recorded_at", Temporal{}, ErrNoRecordedAt},
		{"open start", Temporal{ValidTo: ptr(t2), RecordedAt: t1}, nil},
		{"open end", Temporal{ValidFrom: ptr(t0), RecordedAt: t1}, nil},
		{"window", Temporal{ValidFrom: ptr(t0), ValidTo: ptr(t2), RecordedAt: t1}, nil},
		{"recorded long after validity", Temporal{ValidFrom: ptr(t0), ValidTo: ptr(t1), RecordedAt: t2}, nil},
		{"empty window", Temporal{ValidFrom: ptr(t1), ValidTo: ptr(t1), RecordedAt: t1}, ErrInvalidValidity},
		{"inverted window", Temporal{ValidFrom: ptr(t2), ValidTo: ptr(t0), RecordedAt: t1}, ErrInvalidValidity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.tm.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestTemporalValidAt(t *testing.T) {
	tests := []struct {
		name     string
		from, to *time.Time
		at       time.Time
		want     bool
	}{
		{"unknown validity, any time", nil, nil, t0, true},
		{"before start", ptr(t1), nil, t0, false},
		{"at start", ptr(t1), nil, t1, true},
		{"after start, no known end", ptr(t1), nil, t2.AddDate(10, 0, 0), true},
		{"before end", nil, ptr(t1), t0, true},
		{"at end", nil, ptr(t1), t1, false},
		{"inside window", ptr(t0), ptr(t2), t1, true},
		{"after window", ptr(t0), ptr(t1), t2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := Temporal{ValidFrom: tt.from, ValidTo: tt.to, RecordedAt: t2}
			if got := tm.ValidAt(tt.at); got != tt.want {
				t.Errorf("ValidAt(%v) = %v, want %v", tt.at, got, tt.want)
			}
		})
	}
}

func TestTemporalLastChecked(t *testing.T) {
	if got := (Temporal{RecordedAt: t1}).LastChecked(); !got.Equal(t1) {
		t.Errorf("never verified: LastChecked() = %v, want recorded_at %v", got, t1)
	}
	if got := (Temporal{RecordedAt: t0, VerifiedAt: ptr(t2)}).LastChecked(); !got.Equal(t2) {
		t.Errorf("verified: LastChecked() = %v, want verified_at %v", got, t2)
	}
}

// No code in the package assigns a validity date from anything named
// recorded_at (TR-32): a source-level guard alongside the behavioural tests.
func TestNoValidityFromRecordedAt(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	validity := func(e ast.Expr) bool {
		var name string
		switch x := e.(type) {
		case *ast.Ident:
			name = x.Name
		case *ast.SelectorExpr:
			name = x.Sel.Name
		}
		return name == "ValidFrom" || name == "ValidTo"
	}
	mentionsRecorded := func(e ast.Expr) bool {
		found := false
		ast.Inspect(e, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && strings.EqualFold(id.Name, "recordedAt") {
				found = true
			}
			return !found
		})
		return found
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.KeyValueExpr:
				if validity(x.Key) && mentionsRecorded(x.Value) {
					t.Errorf("%s: validity set from recorded_at", fset.Position(x.Pos()))
				}
			case *ast.AssignStmt:
				for i, lhs := range x.Lhs {
					if validity(lhs) && i < len(x.Rhs) && mentionsRecorded(x.Rhs[i]) {
						t.Errorf("%s: validity set from recorded_at", fset.Position(x.Pos()))
					}
				}
			}
			return true
		})
	}
}
