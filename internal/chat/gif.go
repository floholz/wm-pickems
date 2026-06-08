package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// GIF search proxies KLIPY server-side so the API key never reaches the client,
// and normalises the response into a stable shape ({preview,url,width,height})
// so the frontend is decoupled from KLIPY's exact JSON. Posted GIF URLs are
// validated against a KLIPY host allowlist so a client can't post arbitrary
// links.
//
// NOTE: the exact `files` paths in extractGif and the host allowlist must be
// verified against a real KLIPY response once KLIPY_API_KEY is set — they're
// the only KLIPY-specific bits; everything downstream uses the normalised shape.

var gifAllowedHosts = []string{"klipy.com", "klipy.io", "klipy-cdn.com"}

func gifHostAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range gifAllowedHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

func klipyKey() string { return strings.TrimSpace(os.Getenv("KLIPY_API_KEY")) }

// searchGifs returns trending GIFs (q empty) or search results, normalised.
// Without a key it returns an empty, "configured:false" result so the UI can
// show a friendly state instead of erroring.
func searchGifs(ctx context.Context, q, page string) (map[string]any, error) {
	key := klipyKey()
	if key == "" {
		return map[string]any{"gifs": []any{}, "next": "", "configured": false}, nil
	}
	if page == "" {
		page = "1"
	}
	const perPage = "24"
	var endpoint string
	if strings.TrimSpace(q) == "" {
		endpoint = fmt.Sprintf("https://api.klipy.com/api/v1/%s/gifs/trending?per_page=%s&page=%s&rating=pg-13", key, perPage, page)
	} else {
		endpoint = fmt.Sprintf("https://api.klipy.com/api/v1/%s/gifs/search?q=%s&per_page=%s&page=%s&rating=pg-13",
			key, url.QueryEscape(q), perPage, page)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("klipy: status %d", resp.StatusCode)
	}
	var parsed struct {
		Data struct {
			Data    []map[string]any `json:"data"`
			HasNext bool             `json:"has_next"`
			Page    int              `json:"current_page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	gifs := make([]map[string]any, 0, len(parsed.Data.Data))
	for _, item := range parsed.Data.Data {
		preview, full, w, h := extractGif(item)
		if full == "" {
			continue
		}
		gifs = append(gifs, map[string]any{
			"id":      idStr(item),
			"title":   digStr(item, "title"),
			"preview": preview,
			"url":     full,
			"width":   w,
			"height":  h,
		})
	}
	next := ""
	if parsed.Data.HasNext {
		next = fmt.Sprintf("%d", parsed.Data.Page+1)
	}
	return map[string]any{"gifs": gifs, "next": next, "configured": true}, nil
}

// extractGif pulls (preview, full, w, h) from one KLIPY item.
// KLIPY shape: item.file.{hd,md,sm,xs}.{gif,webp,…}.{url,width,height}.
// The displayed/posted gif uses `sm` (≈220px — matches the in-bubble cap and is
// far lighter than hd/md); the grid preview uses the tiny `xs`.
func extractGif(item map[string]any) (preview, full string, w, h int) {
	full = firstStr(item,
		[]string{"file", "sm", "gif", "url"},
		[]string{"file", "md", "gif", "url"},
		[]string{"file", "hd", "gif", "url"},
	)
	preview = firstStr(item,
		[]string{"file", "xs", "gif", "url"},
		[]string{"file", "sm", "gif", "url"},
	)
	if preview == "" {
		preview = full
	}
	w = intAt(item, "file", "sm", "gif", "width")
	h = intAt(item, "file", "sm", "gif", "height")
	return preview, full, w, h
}

// idStr returns a KLIPY item's id as a string (it comes back as a number),
// falling back to the slug.
func idStr(item map[string]any) string {
	switch v := item["id"].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	}
	return digStr(item, "slug")
}

// --- tiny nested-map helpers ---

func walk(m map[string]any, path []string) any {
	var cur any = m
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func firstStr(m map[string]any, paths ...[]string) string {
	for _, p := range paths {
		if s, ok := walk(m, p).(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func digStr(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func intAt(m map[string]any, path ...string) int {
	switch v := walk(m, path).(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
