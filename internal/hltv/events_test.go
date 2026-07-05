package hltv

import (
	"os"
	"path/filepath"
	"testing"

	"psr/internal/models"
)

func TestIsJunkEventName(t *testing.T) {
	if !IsJunkEventName("Live") {
		t.Fatal("Live should be junk")
	}
	if !IsJunkEventName("Live\nJun 7th") {
		t.Fatal("Live with date should be junk")
	}
	if IsJunkEventName("IEM Cologne Major 2026") {
		t.Fatal("IEM should not be junk")
	}
}

func TestParseEventsBigEvent(t *testing.T) {
	html := `<div class="events-page">
<div class="big-events">
<a href="/events/8301/iem-cologne-major-2026" class="big-event">
<div class="big-event-name">IEM Cologne Major 2026</div>
</a>
</div>
<div class="ongoing-events">
<a href="/events/8300/iem-cologne-stage-2">IEM Cologne Major 2026 Stage 2</a>
</div>
</div>`

	events, err := ParseEvents(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}
	foundMajor := false
	for _, e := range events {
		if e.ID == 8301 {
			foundMajor = true
			if e.Name != "IEM Cologne Major 2026" {
				t.Fatalf("bad name: %q", e.Name)
			}
			if !e.Featured {
				t.Fatal("expected featured major")
			}
			if e.Status != models.EventStatusUpcoming {
				t.Fatalf("expected upcoming, got %s", e.Status)
			}
		}
		if e.ID == 8300 && e.Status != models.EventStatusOngoing {
			t.Fatalf("stage 2 should be ongoing, got %s", e.Status)
		}
	}
	if !foundMajor {
		t.Fatal("IEM Cologne Major 2026 not parsed")
	}
}

func TestParseEvents(t *testing.T) {
	html, err := os.ReadFile(filepath.Join("..", "..", "testdata", "events_sample.html"))
	if err != nil {
		t.Fatal(err)
	}

	events, err := ParseEvents(string(html))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	if events[0].ID != 8048 || events[0].Name == "" {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
}

func TestParseEventsMergeOngoingOverUpcoming(t *testing.T) {
	html := `<div class="events-page">
<div class="big-events">
<a href="/events/8301/iem-cologne-major-2026" class="big-event">
<div class="big-event-name">IEM Cologne Major 2026</div>
</a>
</div>
<div class="ongoing-events">
<a href="/events/8301/iem-cologne-major-2026">IEM Cologne Major 2026</a>
</div>
</div>`

	events, err := ParseEvents(html)
	if err != nil {
		t.Fatal(err)
	}
	var major models.Event
	for _, e := range events {
		if e.ID == 8301 {
			major = e
			break
		}
	}
	if major.ID != 8301 {
		t.Fatal("major not found")
	}
	if major.Status != models.EventStatusOngoing {
		t.Fatalf("expected ongoing, got %s", major.Status)
	}
}

func TestParseTeamSearch(t *testing.T) {
	html := `<a href="/team/4608/natus-vincere">Natus Vincere</a>
<a href="/team/6667/faze">FaZe Clan</a>`

	teams, err := ParseTeamSearch(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
	if teams[0].ID != 4608 {
		t.Fatalf("unexpected team: %+v", teams[0])
	}
}
