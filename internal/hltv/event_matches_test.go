package hltv

import (
	"testing"
	"time"

	"psr/internal/models"
)

func TestParseEventMatchesPage(t *testing.T) {
	html := `<div class="match-wrapper" data-match-id="2394898" team1="9565" team2="13286" data-event-id="8301">
<div class="match-team team1"><span class="match-teamname">Vitality</span></div>
<div class="match-team team2"><span class="match-teamname">BIG</span></div>
<div class="match-meta">Bo3</div>
<div class="match-time" data-unix="1750000000000"></div>
</div>`

	event := models.Event{ID: 8301, Name: "IEM Cologne Major 2026"}
	matches, err := ParseEventMatchesPage(html, event)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	if m.ID != 2394898 {
		t.Fatalf("bad match id: %d", m.ID)
	}
	if m.Team1.ID != 9565 || m.Team1.Name != "Vitality" {
		t.Fatalf("bad team1: %+v", m.Team1)
	}
	if m.Team2.ID != 13286 || m.Team2.Name != "BIG" {
		t.Fatalf("bad team2: %+v", m.Team2)
	}
	if m.Format != models.FormatBO3 {
		t.Fatalf("bad format: %q", m.Format)
	}
	want := time.UnixMilli(1750000000000).UTC()
	if !m.Date.Equal(want) {
		t.Fatalf("bad date: %v want %v", m.Date, want)
	}
}
