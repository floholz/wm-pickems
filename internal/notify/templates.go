package notify

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

//go:embed templates/*.html templates/*.txt
var tmplFS embed.FS

// matchLine is one fixture row in the tips-reminder digest.
type matchLine struct {
	Home     string
	Away     string
	WhenText string
}

// rankLine is one league standing in the results recap.
type rankLine struct {
	League string
	Rank   int
	Of     int
}

// tplData is the union of fields every template might reference. Unused fields
// for a given event are simply left zero. Common fields (AppName, SettingsUrl,
// CTA*) are filled by the dispatcher; the rest by each detector.
type tplData struct {
	AppName     string
	SettingsUrl string
	CTAText     string
	CTAUrl      string

	StageName string
	StartsIn  string
	WhenText  string

	Count   int
	Matches []matchLine

	Finalized    int
	PointsGained int
	Total        int
	Ranks        []rankLine
}

// render produces the subject, HTML body, and text body for an event by
// combining the shared layout with the event's own partials.
func render(event string, data tplData) (subject, html, text string, err error) {
	ht, err := htmltemplate.New("").ParseFS(tmplFS,
		"templates/layout.html", "templates/"+event+".html")
	if err != nil {
		return "", "", "", fmt.Errorf("parse html %s: %w", event, err)
	}
	var sb, hb bytes.Buffer
	if err := ht.ExecuteTemplate(&sb, "subject", data); err != nil {
		return "", "", "", fmt.Errorf("subject %s: %w", event, err)
	}
	if err := ht.ExecuteTemplate(&hb, "layout", data); err != nil {
		return "", "", "", fmt.Errorf("html %s: %w", event, err)
	}

	tt, err := texttemplate.New(event+".txt").ParseFS(tmplFS, "templates/"+event+".txt")
	if err != nil {
		return "", "", "", fmt.Errorf("parse text %s: %w", event, err)
	}
	var tb bytes.Buffer
	if err := tt.Execute(&tb, data); err != nil {
		return "", "", "", fmt.Errorf("text %s: %w", event, err)
	}

	return sb.String(), hb.String(), tb.String(), nil
}

// renderPush builds a push notification's title and body. It prefers the
// push-specific `pushTitle` / `pushBody` blocks (punchier, data-rich copy) and
// falls back to the email `subject` / `preheader` when an event doesn't define
// them. Parsed with text/template (not html/template) so the short plain strings
// aren't HTML-entity escaped.
func renderPush(event string, data tplData) (title, body string, err error) {
	tt, err := texttemplate.New("").ParseFS(tmplFS,
		"templates/layout.html", "templates/"+event+".html")
	if err != nil {
		return "", "", fmt.Errorf("parse push %s: %w", event, err)
	}
	title, err = execBlock(tt, "pushTitle", "subject", data)
	if err != nil {
		return "", "", fmt.Errorf("push title %s: %w", event, err)
	}
	// Body is optional — fall back to empty if neither block renders.
	body, _ = execBlock(tt, "pushBody", "preheader", data)
	return strings.TrimSpace(title), strings.TrimSpace(body), nil
}

// execBlock renders `primary` if defined, else `fallback`.
func execBlock(tt *texttemplate.Template, primary, fallback string, data tplData) (string, error) {
	name := primary
	if tt.Lookup(primary) == nil {
		name = fallback
	}
	var b bytes.Buffer
	if err := tt.ExecuteTemplate(&b, name, data); err != nil {
		return "", err
	}
	return b.String(), nil
}
