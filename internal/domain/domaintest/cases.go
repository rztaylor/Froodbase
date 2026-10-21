package domaintest

import (
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
)

// Day is midnight UTC on the given date, the resolution the tables use.
func Day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func at(t time.Time) *time.Time { return &t }

// OverlapCase is one row of the scope overlap table: whether A and B
// overlap (TR-26). Overlap is symmetric, so a runner checks both orders.
type OverlapCase struct {
	Name string
	A, B domain.Scope
	Want bool
}

// categorical are the scope keys compared as names, with a setter each.
var categorical = []struct {
	key domain.ScopeKey
	set func(*domain.Scope, string)
}{
	{domain.ScopeEnvironment, func(s *domain.Scope, v string) { s.Environment = v }},
	{domain.ScopeRegion, func(s *domain.Scope, v string) { s.Region = v }},
	{domain.ScopeCluster, func(s *domain.Scope, v string) { s.Cluster = v }},
	{domain.ScopeTenant, func(s *domain.Scope, v string) { s.Tenant = v }},
}

func versions(lo, hi string) domain.Scope { return domain.Scope{VersionMin: lo, VersionMax: hi} }

// OverlapCases is the scope overlap table: every combination of specified
// and unspecified for each name key and for both version bounds, the version
// ordering rules, several keys together, and the pitch's records.
func OverlapCases() []OverlapCase {
	var cases []OverlapCase
	for _, c := range categorical {
		with := func(v string) domain.Scope {
			var s domain.Scope
			c.set(&s, v)
			return s
		}
		k := string(c.key)
		cases = append(cases,
			OverlapCase{k + ": both unspecified", with(""), with(""), true},
			OverlapCase{k + ": only A specified", with("prod"), with(""), true},
			OverlapCase{k + ": only B specified", with(""), with("prod"), true},
			OverlapCase{k + ": both specified, equal", with("prod"), with("prod"), true},
			OverlapCase{k + ": both specified, equal ignoring case and spaces", with("Prod"), with(" prod "), true},
			OverlapCase{k + ": both specified, different", with("prod"), with("staging"), false},
			OverlapCase{k + ": blank counts as unspecified", with("   "), with("prod"), true},
		)
	}
	cases = append(cases,
		// Bounds are inclusive; a missing bound is open. Patterns: none, min only, max only, both.
		OverlapCase{"version: none, none", versions("", ""), versions("", ""), true},
		OverlapCase{"version: none, min", versions("", ""), versions("2.3", ""), true},
		OverlapCase{"version: none, max", versions("", ""), versions("", "2.3"), true},
		OverlapCase{"version: none, both", versions("", ""), versions("2.3", "2.3"), true},
		OverlapCase{"version: min, min", versions("2.3", ""), versions("3.0", ""), true},
		OverlapCase{"version: min, max, meeting", versions("2.3", ""), versions("", "2.3"), true},
		OverlapCase{"version: min, max, apart", versions("2.3", ""), versions("", "2.2"), false},
		OverlapCase{"version: min, both, inside", versions("2.3", ""), versions("2.4", "2.4"), true},
		OverlapCase{"version: min, both, below", versions("2.5", ""), versions("2.4", "2.4"), false},
		OverlapCase{"version: max, max", versions("", "2.3"), versions("", "1.0"), true},
		OverlapCase{"version: max, both, meeting", versions("", "2.4"), versions("2.4", "2.4"), true},
		OverlapCase{"version: max, both, above", versions("", "2.3"), versions("2.4", "2.4"), false},
		OverlapCase{"version: both, both, equal", versions("2.3", "2.3"), versions("2.3", "2.3"), true},
		OverlapCase{"version: both, both, apart", versions("2.3", "2.3"), versions("2.4", "2.4"), false},
		OverlapCase{"version: both, both, overlapping ranges", versions("2.0", "2.9"), versions("2.5", "3.5"), true},
		OverlapCase{"version: both, both, adjacent ranges", versions("2.0", "2.4"), versions("2.5", "3.5"), false},
		OverlapCase{"version: both, min, reaching", versions("2.3", "2.3"), versions("2.3", ""), true},
		OverlapCase{"version: both, min, short", versions("2.3", "2.3"), versions("2.4", ""), false},
		OverlapCase{"version: both, max, short", versions("2.3", "2.3"), versions("", "2.2"), false},
		OverlapCase{"version: both, none", versions("2.3", "2.3"), versions("", ""), true},
		// Ordering is numeric per dot-separated segment, not textual.
		OverlapCase{"version: 2.10 is after 2.9", versions("2.10", "2.10"), versions("2.9", "2.9"), false},
		OverlapCase{"version: 2.10 is within 2.9 onwards", versions("2.10", "2.10"), versions("2.9", ""), true},
		OverlapCase{"version: missing segments are zero", versions("2.4", "2.4"), versions("2.4.0", "2.4.0"), true},
		OverlapCase{"version: a leading v is ignored", versions("v2.4", "v2.4"), versions("2.4", "2.4"), true},
		OverlapCase{"version: leading zeros are ignored", versions("2.04", "2.04"), versions("2.4", "2.4"), true},
		// A bound that is not a version cannot rule anything out.
		OverlapCase{"version: an unreadable bound is open", versions("2.4-beta", "2.4-beta"), versions("2.3", "2.3"), true},
		OverlapCase{"version: a readable bound still applies beside an unreadable one", versions("2.5", "latest"), versions("2.4", "2.4"), false},
		// Several keys: every key must overlap.
		OverlapCase{"all keys equal", fullScope(), fullScope(), true},
		OverlapCase{"all keys equal but the tenant", fullScope(), func() domain.Scope { s := fullScope(); s.Tenant = "t2"; return s }(), false},
		OverlapCase{"unspecified against everything specified", domain.Scope{}, fullScope(), true},
		OverlapCase{"environment matches, versions apart", domain.Scope{Environment: "prod", VersionMin: "2.3", VersionMax: "2.3"}, domain.Scope{Environment: "prod", VersionMin: "2.4", VersionMax: "2.4"}, false},
		// The pitch (architecture design section 6).
		OverlapCase{"pitch: failure and merge (repository is not production)", Pitch(PitchFailure).Scope, Pitch(PitchMerge).Scope, false},
		OverlapCase{"pitch: failure and deployment (2.3 is not 2.4)", Pitch(PitchFailure).Scope, Pitch(PitchDeployment).Scope, false},
		OverlapCase{"pitch: failure and imported investigation", Pitch(PitchFailure).Scope, Pitch(PitchImport).Scope, true},
		OverlapCase{"pitch: failure and a search scoped to production", Pitch(PitchFailure).Scope, domain.Scope{Environment: "prod"}, true},
	)
	return cases
}

func fullScope() domain.Scope {
	return domain.Scope{Environment: "prod", Region: "uksouth", VersionMin: "2.3", VersionMax: "2.4", Cluster: "c1", Tenant: "t1"}
}

// The pitch's records (PR-27), named as in architecture design section 6.
const (
	PitchFailure    = "failure"    // callback failure reproduced in production on 2.3
	PitchMerge      = "merge"      // fix merged (PR 418)
	PitchDeployment = "deployment" // 2.4 deployed, remediation verified
	PitchImport     = "import"     // older investigation imported later
)

// PitchRecord is one of the pitch's records as stored, with the
// lifecycle state it holds once the deployment supersedes the failure.
type PitchRecord struct {
	Name   string
	Body   string
	Scope  domain.Scope
	Record domain.ModeRecord
}

// The pitch's dates.
var (
	pitchIncident   = Day(2026, 9, 2)
	pitchMerged     = Day(2026, 9, 5)
	pitchDeployed   = Day(2026, 9, 9)
	pitchImported   = Day(2026, 10, 13)
	pitchQueriedNow = Day(2026, 10, 14)
)

// PitchRecords returns the four records of the worked example. The failure is
// superseded by the deployment with its validity unchanged (TR-33): nobody
// asserted an end date, so valid_to stays null. The imported investigation
// is recorded on 13 October about 2 to 9 September.
func PitchRecords() []PitchRecord {
	return []PitchRecord{
		{
			Name: PitchFailure, Body: "Callback failure reproduced in production on version 2.3",
			Scope: domain.Scope{Environment: "prod", VersionMin: "2.3", VersionMax: "2.3"},
			Record: domain.ModeRecord{Kind: domain.KindObservation, State: domain.LifecycleSuperseded,
				Temporal: domain.Temporal{ValidFrom: at(pitchIncident), RecordedAt: pitchIncident, VerifiedAt: at(pitchIncident)}},
		},
		{
			Name: PitchMerge, Body: "Fix for the callback failure merged in PR 418",
			Scope: domain.Scope{Environment: "repository"},
			Record: domain.ModeRecord{Kind: domain.KindObservation, State: domain.LifecycleActive,
				Temporal: domain.Temporal{ValidFrom: at(pitchMerged), RecordedAt: pitchMerged}},
		},
		{
			Name: PitchDeployment, Body: "Version 2.4 deployed to production and the callback remediation verified",
			Scope: domain.Scope{Environment: "prod", VersionMin: "2.4", VersionMax: "2.4"},
			Record: domain.ModeRecord{Kind: domain.KindObservation, State: domain.LifecycleActive,
				Temporal: domain.Temporal{ValidFrom: at(pitchDeployed), RecordedAt: pitchDeployed, VerifiedAt: at(pitchDeployed)}},
		},
		{
			Name: PitchImport, Body: "The earlier investigation concluded the callbacks failed on 2.3 until the fix",
			Scope: domain.Scope{Environment: "prod", VersionMin: "2.3", VersionMax: "2.3"},
			Record: domain.ModeRecord{Kind: domain.KindInference, State: domain.LifecycleActive,
				Temporal: domain.Temporal{ValidFrom: at(pitchIncident), ValidTo: at(pitchDeployed), RecordedAt: pitchImported}},
		},
	}
}

// Pitch returns the pitch record with the given name; it panics on an
// unknown name, which is a mistake in a test.
func Pitch(name string) PitchRecord {
	for _, r := range PitchRecords() {
		if r.Name == name {
			return r
		}
	}
	panic("domaintest: no pitch record " + name)
}

// ModeCase is one row of the mode table: a record, a query, the reference
// time (now) and the area's freshness policy, and the expected result:
// whether the mode keeps the record, its status relative to now, and
// whether it is stale.
type ModeCase struct {
	Name   string
	Record domain.ModeRecord
	Query  domain.Query
	Now    time.Time
	Policy domain.FreshnessPolicy
	Want   domain.ModeResult
}

// ModeCases is the mode table: the pitch's records at several times, every
// lifecycle state against every shape of validity window in each mode, the
// window boundaries, and the stale rule at the freshness policy's edge.
func ModeCases() []ModeCase {
	var cases []ModeCase
	cases = append(cases, pitchCases()...)
	cases = append(cases, matrixCases()...)
	cases = append(cases, boundaryCases()...)
	cases = append(cases, staleCases()...)
	return cases
}

func result(included bool, status domain.TemporalStatus, stale bool) domain.ModeResult {
	return domain.ModeResult{Included: included, Status: status, Stale: stale}
}

const (
	current    = domain.StatusCurrent
	historical = domain.StatusHistorical
	upcoming   = domain.StatusUpcoming
)

func pitchCases() []ModeCase {
	// Status relative to 14 October: the failure is superseded and the
	// import's validity ended on 9 September, so both are historical.
	status := map[string]domain.TemporalStatus{PitchFailure: historical, PitchMerge: current, PitchDeployment: current, PitchImport: historical}
	kept := []struct {
		label string
		query domain.Query
		in    map[string]bool
	}{
		{"current", domain.CurrentQuery(), map[string]bool{PitchMerge: true, PitchDeployment: true}},
		{"as_of 1 Sep, before anything", domain.AsOfQuery(Day(2026, 9, 1)), map[string]bool{}},
		{"as_of 3 Sep, the incident", domain.AsOfQuery(Day(2026, 9, 3)), map[string]bool{PitchFailure: true, PitchImport: true}},
		{"as_of 6 Sep, after the merge", domain.AsOfQuery(Day(2026, 9, 6)), map[string]bool{PitchFailure: true, PitchMerge: true, PitchImport: true}},
		{"as_of 9 Sep, the deployment", domain.AsOfQuery(pitchDeployed), map[string]bool{PitchFailure: true, PitchMerge: true, PitchDeployment: true}},
		{"as_of 10 Sep", domain.AsOfQuery(Day(2026, 9, 10)), map[string]bool{PitchFailure: true, PitchMerge: true, PitchDeployment: true}},
		{"historical", domain.HistoricalQuery(), map[string]bool{PitchFailure: true, PitchMerge: true, PitchDeployment: true, PitchImport: true}},
	}
	var cases []ModeCase
	for _, k := range kept {
		for _, r := range PitchRecords() {
			cases = append(cases, ModeCase{
				Name:   "pitch " + k.label + ": " + r.Name,
				Record: r.Record, Query: k.query, Now: pitchQueriedNow, Policy: domain.DefaultFreshnessPolicy(),
				Want: result(k.in[r.Name], status[r.Name], false),
			})
		}
	}
	// By 31 December neither live record has been checked for more than 90
	// days: both stay current and are flagged stale, not dropped.
	late := Day(2026, 12, 31)
	for _, r := range PitchRecords() {
		in := r.Name == PitchMerge || r.Name == PitchDeployment
		cases = append(cases, ModeCase{
			Name:   "pitch current on 31 Dec: " + r.Name,
			Record: r.Record, Query: domain.CurrentQuery(), Now: late, Policy: domain.DefaultFreshnessPolicy(),
			Want: result(in, status[r.Name], in),
		})
	}
	return cases
}

// matrixCases checks every lifecycle state against every shape of validity
// window, with now on 1 June 2026 and as_of on 1 February 2026.
func matrixCases() []ModeCase {
	var (
		now   = Day(2026, 6, 1)
		asOf  = Day(2026, 2, 1)
		jan1  = Day(2026, 1, 1)
		mar1  = Day(2026, 3, 1)
		apr1  = Day(2026, 4, 1)
		dec1  = Day(2026, 12, 1)
		fresh = now.Add(-24 * time.Hour)
	)
	// For each window: what an active or disputed record does in current
	// mode and its status, and whether its validity includes the as_of time.
	shapes := []struct {
		name     string
		from, to *time.Time
		current  bool
		status   domain.TemporalStatus
		asOf     bool
	}{
		{"validity unknown", nil, nil, true, current, true},
		{"started, no known end", at(jan1), nil, true, current, true},
		{"window around now", at(jan1), at(dec1), true, current, true},
		{"ended before now", at(jan1), at(mar1), false, historical, true},
		{"starts after now", at(dec1), nil, false, upcoming, false},
		{"start unknown, ends after now", nil, at(dec1), true, current, true},
		{"start unknown, ended before as_of", nil, at(jan1), false, historical, false},
		{"started between as_of and now", at(apr1), nil, true, current, false},
	}
	var cases []ModeCase
	for _, sh := range shapes {
		for _, st := range domain.LifecycleStates() {
			live := st == domain.LifecycleActive || st == domain.LifecycleDisputed
			status := historical
			if live {
				status = sh.status
			}
			rec := domain.ModeRecord{Kind: domain.KindObservation, State: st,
				Temporal: domain.Temporal{ValidFrom: sh.from, ValidTo: sh.to, RecordedAt: now, VerifiedAt: at(fresh)}}
			name := string(st) + ", " + sh.name
			cases = append(cases,
				ModeCase{"current: " + name, rec, domain.CurrentQuery(), now, domain.DefaultFreshnessPolicy(),
					result(live && sh.current, status, false)},
				ModeCase{"as_of: " + name, rec, domain.AsOfQuery(asOf), now, domain.DefaultFreshnessPolicy(),
					result(st != domain.LifecycleRetracted && sh.asOf, status, false)},
				ModeCase{"historical: " + name, rec, domain.HistoricalQuery(), now, domain.DefaultFreshnessPolicy(),
					result(true, status, false)},
			)
		}
	}
	return cases
}

// boundaryCases pin the half-open window [valid_from, valid_to) and that
// recording time plays no part (TR-34).
func boundaryCases() []ModeCase {
	var (
		from = Day(2026, 3, 1)
		to   = Day(2026, 9, 1)
		p    = domain.DefaultFreshnessPolicy()
	)
	rec := func(recorded time.Time) domain.ModeRecord {
		return domain.ModeRecord{Kind: domain.KindDecision, State: domain.LifecycleActive,
			Temporal: domain.Temporal{ValidFrom: at(from), ValidTo: at(to), RecordedAt: recorded}}
	}
	return []ModeCase{
		{"boundary: current at valid_from", rec(from), domain.CurrentQuery(), from, p, result(true, current, false)},
		{"boundary: current at valid_to", rec(from), domain.CurrentQuery(), to, p, result(false, historical, false)},
		{"boundary: current a microsecond before valid_from", rec(Day(2026, 1, 1)), domain.CurrentQuery(), from.Add(-time.Microsecond), p, result(false, upcoming, false)},
		{"boundary: as_of at valid_from", rec(from), domain.AsOfQuery(from), to, p, result(true, historical, false)},
		{"boundary: as_of a microsecond before valid_to", rec(from), domain.AsOfQuery(to.Add(-time.Microsecond)), to, p, result(true, historical, false)},
		{"boundary: as_of at valid_to", rec(from), domain.AsOfQuery(to), to, p, result(false, historical, false)},
		{"recorded after its validity, as_of inside it", rec(Day(2026, 10, 1)), domain.AsOfQuery(Day(2026, 5, 1)), Day(2026, 10, 2), p, result(true, historical, false)},
		{"recorded after its validity, current", rec(Day(2026, 10, 1)), domain.CurrentQuery(), Day(2026, 10, 2), p, result(false, historical, false)},
	}
}

// staleCases pin the stale rule: a record current now whose last check
// (verified_at, or recorded_at if it was never verified) is older than its
// kind's maximum age is stale and still returned (TR-31).
func staleCases() []ModeCase {
	now := Day(2026, 12, 1)
	days := func(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour) }
	live := func(kind domain.EpistemicKind, recorded time.Time, verified *time.Time) domain.ModeRecord {
		return domain.ModeRecord{Kind: kind, State: domain.LifecycleActive,
			Temporal: domain.Temporal{ValidFrom: at(Day(2025, 1, 1)), RecordedAt: recorded, VerifiedAt: verified}}
	}
	def := domain.DefaultFreshnessPolicy()
	monthly := domain.FreshnessPolicy{MaxAgeDays: map[domain.EpistemicKind]int{domain.KindObservation: 30, domain.KindDecision: 7}}
	noLimit := domain.FreshnessPolicy{}
	obs, dec, inf := domain.KindObservation, domain.KindDecision, domain.KindInference
	cq := domain.CurrentQuery()
	return []ModeCase{
		{"stale: observation verified exactly 90 days ago", live(obs, days(400), at(days(90))), cq, now, def, result(true, current, false)},
		{"stale: observation verified 90 days and a second ago", live(obs, days(400), at(days(90).Add(-time.Second))), cq, now, def, result(true, current, true)},
		{"stale: observation verified 89 days ago", live(obs, days(400), at(days(89))), cq, now, def, result(true, current, false)},
		{"stale: inference verified 91 days ago", live(inf, days(400), at(days(91))), cq, now, def, result(true, current, true)},
		{"stale: decision verified 1000 days ago has no limit", live(dec, days(1000), at(days(1000))), cq, now, def, result(true, current, false)},
		{"stale: never verified, recorded 91 days ago", live(obs, days(91), nil), cq, now, def, result(true, current, true)},
		{"stale: never verified, recorded 10 days ago", live(obs, days(10), nil), cq, now, def, result(true, current, false)},
		{"stale: verified 10 days ago, recorded 400 days ago", live(obs, days(400), at(days(10))), cq, now, def, result(true, current, false)},
		{"stale: verified after now (clock skew)", live(obs, days(400), at(now.Add(time.Hour))), cq, now, def, result(true, current, false)},
		{"stale: in historical mode too", live(obs, days(400), at(days(200))), domain.HistoricalQuery(), now, def, result(true, current, true)},
		{"stale: in as_of mode, relative to now", live(obs, days(400), at(days(200))), domain.AsOfQuery(Day(2025, 6, 1)), now, def, result(true, current, true)},
		{"stale: not for a record whose validity ended", domain.ModeRecord{Kind: obs, State: domain.LifecycleActive,
			Temporal: domain.Temporal{ValidFrom: at(Day(2025, 1, 1)), ValidTo: at(Day(2025, 6, 1)), RecordedAt: days(400), VerifiedAt: at(days(400))}},
			domain.HistoricalQuery(), now, def, result(true, historical, false)},
		{"stale: not for a superseded record", domain.ModeRecord{Kind: obs, State: domain.LifecycleSuperseded,
			Temporal: domain.Temporal{ValidFrom: at(Day(2025, 1, 1)), RecordedAt: days(400), VerifiedAt: at(days(400))}},
			domain.HistoricalQuery(), now, def, result(true, historical, false)},
		{"stale: disputed records go stale too", domain.ModeRecord{Kind: obs, State: domain.LifecycleDisputed,
			Temporal: domain.Temporal{RecordedAt: days(400), VerifiedAt: at(days(100))}},
			cq, now, def, result(true, current, true)},
		{"stale: area policy of 30 days for observations", live(obs, days(400), at(days(31))), cq, now, monthly, result(true, current, true)},
		{"stale: area policy of 7 days for decisions", live(dec, days(400), at(days(8))), cq, now, monthly, result(true, current, true)},
		{"stale: area policy without inferences has no limit for them", live(inf, days(400), at(days(400))), cq, now, monthly, result(true, current, false)},
		{"stale: area policy with no limits", live(obs, days(400), at(days(400))), cq, now, noLimit, result(true, current, false)},
	}
}
