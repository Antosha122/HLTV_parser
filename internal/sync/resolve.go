package sync

import (
	"context"
	"fmt"
	"strings"

	"psr/internal/models"
)

func (s *Service) resolveTeam(ctx context.Context, t models.Team) (models.Team, error) {
	if t.ID > 0 {
		if t.Name != "" {
			_ = s.db.UpsertTeam(t)
			return t, nil
		}
		detail, err := s.client.GetTeam(ctx, t.ID)
		if err != nil {
			return t, err
		}
		if err := s.saveTeam(detail); err != nil {
			return models.Team{}, err
		}
		return detail.Team, nil
	}
	if t.Name == "" {
		return models.Team{}, fmt.Errorf("empty team")
	}

	name := strings.TrimSpace(t.Name)
	if teams, err := s.db.ListTeams(name, 15); err == nil {
		for _, hit := range teams {
			if strings.EqualFold(hit.Name, name) {
				return hit, nil
			}
		}
	}

	// Без HLTV id — не вызываем /search?term= (JSON API, не работает по HTTP+cookie).
	return models.Team{}, fmt.Errorf("team not found: %s (no id)", name)
}

func (s *Service) ensureTeams(ctx context.Context, teams ...models.Team) error {
	for _, t := range teams {
		if t.ID > 0 && t.Name != "" {
			if err := s.db.UpsertTeam(t); err != nil {
				return err
			}
			continue
		}
		resolved, err := s.resolveTeam(ctx, t)
		if err != nil {
			return err
		}
		if err := s.db.UpsertTeam(resolved); err != nil {
			return err
		}
	}
	return nil
}
