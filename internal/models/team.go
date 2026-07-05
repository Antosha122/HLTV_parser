package models

type Team struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	HLTVRating float64 `json:"hltv_rating,omitempty"`
	WorldRank  int     `json:"world_rank,omitempty"`
}
