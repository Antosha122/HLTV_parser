package hltv

import (
	"testing"
)

func TestParseRankingPage(t *testing.T) {
	html := `<div class="ranked-team standard-box">
<a href="/team/9565/vitality" class="moreLink"></a>
<span class="position">#1</span>
<span class="name">Vitality</span>
<span class="points">(1000)</span>
</div>
<div class="ranked-team standard-box">
<a href="/team/4608/natus-vincere" class="moreLink"></a>
<span class="position">#2</span>
<span class="name">Natus Vincere</span>
<span class="points">(950)</span>
</div>`

	teams, err := ParseRankingPage(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
	if teams[0].ID != 9565 || teams[0].Name != "Vitality" || teams[0].WorldRank != 1 {
		t.Fatalf("unexpected first team: %+v", teams[0])
	}
	if teams[1].ID != 4608 || teams[1].WorldRank != 2 {
		t.Fatalf("unexpected second team: %+v", teams[1])
	}
}
