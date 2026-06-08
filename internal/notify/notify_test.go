package notify

import (
	"strings"
	"testing"
	"time"
)

func TestInLeadWindow(t *testing.T) {
	now := time.Date(2026, 6, 11, 6, 0, 0, 0, time.UTC)
	lead := 12 * time.Hour
	tests := []struct {
		name  string
		start time.Time
		want  bool
	}{
		{"in the past", now.Add(-time.Hour), false},
		{"now exactly (not future)", now, false},
		{"just inside future", now.Add(time.Minute), true},
		{"mid-window", now.Add(6 * time.Hour), true},
		{"at the lead edge", now.Add(12 * time.Hour), true},
		{"just past the lead edge", now.Add(12*time.Hour + time.Minute), false},
		{"far future", now.Add(48 * time.Hour), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := inLeadWindow(now, tc.start, lead); got != tc.want {
				t.Fatalf("inLeadWindow(%v) = %v, want %v", tc.start.Sub(now), got, tc.want)
			}
		})
	}
}

func TestHumanizeDur(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "less than an hour"},
		{59 * time.Minute, "less than an hour"},
		{time.Hour, "1 hours"},
		{90 * time.Minute, "2 hours"}, // rounds to nearest hour
		{11*time.Hour + 40*time.Minute, "12 hours"},
		{47 * time.Hour, "47 hours"},
		{48 * time.Hour, "2 days"},
		{60 * time.Hour, "3 days"}, // 2.5d rounds up
	}
	for _, tc := range tests {
		if got := humanizeDur(tc.d); got != tc.want {
			t.Errorf("humanizeDur(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestPrefEnabledFromRaw(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		event   string
		channel string
		want    bool
	}{
		{"empty prefs default on", "", "tips_reminder", "email", true},
		{"invalid json default on", "{not json", "tips_reminder", "email", true},
		{"event absent default on", `{"stage_starting":{"email":true}}`, "tips_reminder", "email", true},
		{"channel key absent default on", `{"tips_reminder":{}}`, "tips_reminder", "email", true},
		{"email explicitly on", `{"tips_reminder":{"email":true}}`, "tips_reminder", "email", true},
		{"email explicitly off", `{"tips_reminder":{"email":false}}`, "tips_reminder", "email", false},
		{"off for a different event only", `{"results_recap":{"email":false}}`, "tips_reminder", "email", true},
		{"push off while email on", `{"tips_reminder":{"email":true,"push":false}}`, "tips_reminder", "push", false},
		{"push on while email off", `{"tips_reminder":{"email":false,"push":true}}`, "tips_reminder", "push", true},
		{"push absent default on", `{"tips_reminder":{"email":false}}`, "tips_reminder", "push", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := prefEnabledFromRaw(tc.raw, tc.event, tc.channel); got != tc.want {
				t.Fatalf("prefEnabledFromRaw(%q,%q,%q) = %v, want %v", tc.raw, tc.event, tc.channel, got, tc.want)
			}
		})
	}
}

func TestToPath(t *testing.T) {
	tests := map[string]string{
		"https://fhdt.example.ts.net/tips":          "/tips",
		"https://prod.example.com/forecast?u=abc#x": "/forecast?u=abc",
		"http://localhost:8090/leagues":             "/leagues",
		"/settings":                                 "/settings",
		"/tips?x=1":                                 "/tips?x=1",
	}
	for in, want := range tests {
		if got := toPath(in); got != want {
			t.Errorf("toPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderPushTipsCompact(t *testing.T) {
	// A single untipped match should produce the short "CODE vs CODE" title and
	// a status-first body (so the collapsed notification reads well).
	data := tplData{
		Count: 1,
		Matches: []matchLine{{
			Home: "Mexico", Away: "South Africa",
			HomeCode: "MEX", AwayCode: "RSA",
			WhenText: "Thu, Jun 11 · 19:00 UTC",
		}},
	}
	title, body, err := renderPush("tips_reminder", data)
	if err != nil {
		t.Fatal(err)
	}
	if title != "MEX vs RSA" {
		t.Fatalf("title = %q, want %q", title, "MEX vs RSA")
	}
	if !strings.HasPrefix(body, "Not yet tipped") {
		t.Fatalf("body = %q, want it to lead with %q", body, "Not yet tipped")
	}
	t.Logf("compact tips → title=%q body=%q", title, body)
}

func TestRenderPushAllEvents(t *testing.T) {
	events := []string{"stage_starting", "forecast_reminder", "tips_reminder", "results_recap", "announcement"}
	data := tplData{
		StageName: "Round of 32", StartsIn: "12 hours", WhenText: "Sat 18:00 UTC",
		Count: 2, Matches: []matchLine{{Home: "Brazil", Away: "Spain", WhenText: "soon"}},
		Finalized: 3, PointsGained: 7, Total: 42,
		Title: "Heads up", Body: "We shipped a new feature.",
	}
	for _, ev := range events {
		t.Run(ev, func(t *testing.T) {
			title, body, err := renderPush(ev, data)
			if err != nil {
				t.Fatalf("renderPush(%s): %v", ev, err)
			}
			if title == "" {
				t.Fatalf("renderPush(%s): empty title", ev)
			}
			t.Logf("%s → title=%q body=%q", ev, title, body)
		})
	}
}

func intp(v int) *int { return &v }

func TestApplyConfigDefaults(t *testing.T) {
	tests := []struct {
		name        string
		in          storedConfig
		wantLead    int
		wantRecapHr int
	}{
		{"all unset -> defaults", storedConfig{}, defaultLeadHours, defaultRecapHourUTC},
		{"lead set", storedConfig{LeadHours: intp(6)}, 6, defaultRecapHourUTC},
		{"lead zero ignored", storedConfig{LeadHours: intp(0)}, defaultLeadHours, defaultRecapHourUTC},
		{"lead negative ignored", storedConfig{LeadHours: intp(-3)}, defaultLeadHours, defaultRecapHourUTC},
		{"recap midnight honored", storedConfig{RecapHourUTC: intp(0)}, defaultLeadHours, 0},
		{"recap set", storedConfig{RecapHourUTC: intp(20)}, defaultLeadHours, 20},
		{"recap out of range ignored", storedConfig{RecapHourUTC: intp(25)}, defaultLeadHours, defaultRecapHourUTC},
		{"both set", storedConfig{LeadHours: intp(24), RecapHourUTC: intp(9)}, 24, 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := applyConfigDefaults(tc.in)
			if got.LeadHours != tc.wantLead || got.RecapHourUTC != tc.wantRecapHr {
				t.Fatalf("applyConfigDefaults(%+v) = %+v, want lead=%d recap=%d",
					tc.in, got, tc.wantLead, tc.wantRecapHr)
			}
		})
	}
}

func TestNormalizeEmails(t *testing.T) {
	got := normalizeEmails([]string{"  Me@Example.com ", "", "  ", "x@Y.COM"})
	want := []string{"me@example.com", "x@y.com"}
	if len(got) != len(want) {
		t.Fatalf("normalizeEmails len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeEmails[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestApplyConfigDefaultsAllowlist(t *testing.T) {
	t.Setenv("NOTIFY_ALLOWLIST", "")
	got := applyConfigDefaults(storedConfig{Allowlist: []string{"A@B.com", " ", "c@d.com "}})
	if len(got.Allowlist) != 2 || got.Allowlist[0] != "a@b.com" || got.Allowlist[1] != "c@d.com" {
		t.Fatalf("allowlist = %v, want [a@b.com c@d.com]", got.Allowlist)
	}

	// Falls back to the env seed when the stored list is empty.
	t.Setenv("NOTIFY_ALLOWLIST", "Foo@Bar.com, baz@qux.com")
	got = applyConfigDefaults(storedConfig{})
	if len(got.Allowlist) != 2 || got.Allowlist[0] != "foo@bar.com" || got.Allowlist[1] != "baz@qux.com" {
		t.Fatalf("env allowlist = %v, want [foo@bar.com baz@qux.com]", got.Allowlist)
	}
}

func TestRenderAllTemplates(t *testing.T) {
	// Each event template must parse and execute against tplData without error,
	// and produce a non-empty subject + bodies.
	events := []string{"stage_starting", "forecast_reminder", "tips_reminder", "results_recap"}
	data := tplData{
		AppName:      "WM Pickems",
		SettingsUrl:  "https://example.test/settings",
		CTAText:      "Go",
		CTAUrl:       "https://example.test/tips",
		StageName:    "Round of 32",
		StartsIn:     "12 hours",
		WhenText:     "Sat, Jun 28 · 18:00 UTC",
		Count:        2,
		Matches:      []matchLine{{Home: "Brazil", Away: "Spain", WhenText: "soon"}},
		Finalized:    3,
		PointsGained: 7,
		Total:        42,
		Ranks:        []rankLine{{League: "Friends", Rank: 1, Of: 8}},
	}
	for _, ev := range events {
		t.Run(ev, func(t *testing.T) {
			subject, html, text, err := render(ev, data)
			if err != nil {
				t.Fatalf("render(%s): %v", ev, err)
			}
			if subject == "" || html == "" || text == "" {
				t.Fatalf("render(%s): empty output (subj=%q htmlLen=%d textLen=%d)",
					ev, subject, len(html), len(text))
			}
		})
	}
}
