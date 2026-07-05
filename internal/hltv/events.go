package hltv

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"psr/internal/models"
)

func (c *Client) GetEvents(ctx context.Context) ([]models.Event, error) {
	html, err := c.Fetch(ctx, "/events")
	if err != nil {
		return nil, err
	}
	events, err := ParseEvents(html)
	if err != nil {
		return nil, ErrHLTVUnavailable
	}
	return events, nil
}

func ParseEvents(html string) ([]models.Event, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	seen := make(map[int]int)
	var events []models.Event

	add := func(e models.Event) {
		e.Name = cleanEventName(e.Name)
		if e.ID <= 0 || e.Name == "" || IsJunkEventName(e.Name) {
			return
		}
		if idx, dup := seen[e.ID]; dup {
			events[idx] = mergeParsedEvent(events[idx], e)
			return
		}
		seen[e.ID] = len(events)
		events = append(events, e)
	}

	// Featured majors (IEM Cologne etc.) — use clean name from .big-event-name.
	doc.Find("a.big-event").Each(func(_ int, s *goquery.Selection) {
		id, name, ok := parseEventLink(s)
		if !ok {
			return
		}
		name = strings.TrimSpace(s.Find(".big-event-name").First().Text())
		if name == "" {
			name = eventNameFromHref(s)
		}
		add(models.Event{
			ID:       id,
			Name:     name,
			Status:   detectBigEventStatus(s),
			Featured: true,
		})
	})

	// Ongoing strip at the top of the events page.
	doc.Find(".ongoing-events a[href^='/events/']").Each(func(_ int, s *goquery.Selection) {
		id, name, ok := parseEventLink(s)
		if !ok {
			return
		}
		if n := strings.TrimSpace(s.Text()); n != "" && len(n) < len(name)+40 {
			name = n
		}
		add(models.Event{
			ID:     id,
			Name:   name,
			Status: models.EventStatusOngoing,
		})
	})

	// All other event links.
	doc.Find("a[href^='/events/']").Each(func(_ int, s *goquery.Selection) {
		id, name, ok := parseEventLink(s)
		if !ok {
			return
		}
		if n := strings.TrimSpace(s.Text()); n != "" && len(n) < 120 {
			name = n
		}
		add(models.Event{
			ID:     id,
			Name:   name,
			Status: detectEventStatus(s),
		})
	})

	if len(events) == 0 {
		return nil, fmt.Errorf("no events found in page (layout may have changed)")
	}
	return events, nil
}

func parseEventLink(s *goquery.Selection) (id int, name string, ok bool) {
	href, exists := s.Attr("href")
	if !exists || !strings.HasPrefix(href, "/events/") {
		return 0, "", false
	}
	parts := strings.Split(strings.Trim(href, "/"), "/")
	if len(parts) < 2 || parts[0] != "events" {
		return 0, "", false
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil || id <= 0 {
		return 0, "", false
	}
	name = eventNameFromHref(s)
	if name == "" {
		name = strings.TrimSpace(s.Text())
	}
	return id, name, true
}

func eventNameFromHref(s *goquery.Selection) string {
	href, _ := s.Attr("href")
	parts := strings.Split(strings.Trim(href, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	return strings.ReplaceAll(parts[2], "-", " ")
}

func detectBigEventStatus(s *goquery.Selection) models.EventStatus {
	if s.Closest(".ongoing-events, .ongoing-event").Length() > 0 {
		return models.EventStatusOngoing
	}
	if status := eventStatusFromDates(s); status != "" {
		return status
	}
	return models.EventStatusUpcoming
}

func eventStatusFromDates(s *goquery.Selection) models.EventStatus {
	block := s.Closest(".big-event, .content.standard-box, .event-box")
	if block.Length() == 0 {
		block = s
	}
	var starts []time.Time
	block.Find("[data-unix]").Each(func(_ int, el *goquery.Selection) {
		if t := ParseEventDate(el.AttrOr("data-unix", "")); !t.IsZero() {
			starts = append(starts, t)
		}
	})
	if len(starts) == 0 {
		return ""
	}
	start := starts[0]
	end := starts[len(starts)-1]
	now := time.Now().UTC()
	if !now.Before(start) && !now.After(end.Add(24*time.Hour)) {
		return models.EventStatusOngoing
	}
	if now.Before(start) {
		return models.EventStatusUpcoming
	}
	return models.EventStatusPast
}

func mergeParsedEvent(existing, newer models.Event) models.Event {
	out := existing
	if eventStatusRank(newer.Status) > eventStatusRank(existing.Status) {
		out.Status = newer.Status
	}
	if newer.Featured {
		out.Featured = true
	}
	if pickEventName(existing.Name, newer.Name) == newer.Name {
		out.Name = newer.Name
	}
	return out
}

func eventStatusRank(s models.EventStatus) int {
	switch s {
	case models.EventStatusOngoing:
		return 3
	case models.EventStatusUpcoming:
		return 2
	case models.EventStatusPast:
		return 1
	default:
		return 0
	}
}

func pickEventName(a, b string) string {
	a = cleanEventName(a)
	b = cleanEventName(b)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	aLow, bLow := strings.ToLower(a), strings.ToLower(b)
	if strings.Contains(aLow, "stage") && !strings.Contains(bLow, "stage") {
		return b
	}
	if strings.Contains(bLow, "stage") && !strings.Contains(aLow, "stage") {
		return a
	}
	if len(b) < len(a) {
		return b
	}
	return a
}

func detectEventStatus(s *goquery.Selection) models.EventStatus {
	container := s.Closest(".ongoing-event, .upcoming-event, .past-event, .col-box")
	class, _ := container.Attr("class")
	switch {
	case strings.Contains(class, "ongoing"):
		return models.EventStatusOngoing
	case strings.Contains(class, "upcoming"):
		return models.EventStatusUpcoming
	case strings.Contains(class, "past"):
		return models.EventStatusPast
	default:
		if s.Closest(".ongoing-events").Length() > 0 {
			return models.EventStatusOngoing
		}
		if s.Closest(".big-events").Length() > 0 {
			return models.EventStatusUpcoming
		}
		parent := s.ParentsFiltered(".events-page-wrap, body").First()
		section := s.ParentsUntil(".events-page-wrap").Last()
		sectionText := strings.ToLower(section.Text() + parent.Text())
		if strings.Contains(sectionText, "ongoing") {
			return models.EventStatusOngoing
		}
		if strings.Contains(sectionText, "upcoming") {
			return models.EventStatusUpcoming
		}
		return models.EventStatusUpcoming
	}
}

func cleanEventName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.IndexAny(name, "\n\r"); i >= 0 {
		name = strings.TrimSpace(name[:i])
	}
	return name
}

// IsJunkEventName filters sidebar/widget links that are not real tournaments.
func IsJunkEventName(name string) bool {
	name = cleanEventName(name)
	low := strings.ToLower(name)
	if len(low) < 4 {
		return true
	}
	if strings.HasPrefix(low, "live") {
		return true
	}
	switch low {
	case "more", "results", "matches", "news", "forum", "betting", "watch", "vod":
		return true
	}
	return false
}

// ParseEventDate parses HLTV unix timestamp attributes (seconds or milliseconds).
func ParseEventDate(unix string) time.Time {
	ms, err := strconv.ParseInt(strings.TrimSpace(unix), 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}
	}
	if ms > 1_000_000_000_000 {
		return time.UnixMilli(ms).UTC()
	}
	return time.Unix(ms, 0).UTC()
}
