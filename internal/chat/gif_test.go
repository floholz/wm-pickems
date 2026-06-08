package chat

import (
	"encoding/json"
	"testing"
)

// sampleKlipyItem is a trimmed-but-faithful KLIPY trending item (real response
// shape: file.{hd,md,sm,xs}.{gif,…}.{url,width,height}; id is a number).
const sampleKlipyItem = `{
  "id": 3090582259040157,
  "slug": "eepy-cat-kitten",
  "title": "Adorable Eepy Cat Kitten Yawns",
  "file": {
    "hd": { "gif": { "url": "https://static.klipy.com/x/hd.gif", "width": 498, "height": 428 } },
    "md": { "gif": { "url": "https://static.klipy.com/x/md.gif", "width": 498, "height": 428 } },
    "sm": { "gif": { "url": "https://static.klipy.com/x/sm.gif", "width": 220, "height": 190 } },
    "xs": { "gif": { "url": "https://static.klipy.com/x/xs.gif", "width": 105, "height": 90 } }
  },
  "type": "gif"
}`

func TestExtractGif(t *testing.T) {
	var item map[string]any
	if err := json.Unmarshal([]byte(sampleKlipyItem), &item); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := idStr(item); got != "3090582259040157" {
		t.Errorf("idStr = %q, want the numeric id as a string", got)
	}

	preview, full, w, h := extractGif(item)
	if full != "https://static.klipy.com/x/sm.gif" {
		t.Errorf("full url = %q, want the sm.gif", full)
	}
	if preview != "https://static.klipy.com/x/xs.gif" {
		t.Errorf("preview url = %q, want the xs.gif", preview)
	}
	if w != 220 || h != 190 {
		t.Errorf("dims = %dx%d, want 220x190 (sm)", w, h)
	}

	if !gifHostAllowed(full) {
		t.Errorf("gifHostAllowed(%q) = false, want true (static.klipy.com)", full)
	}
	if gifHostAllowed("https://evil.example.com/x.gif") {
		t.Error("gifHostAllowed allowed a non-KLIPY host")
	}
}
