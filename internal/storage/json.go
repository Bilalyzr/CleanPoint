package storage

import (
	"encoding/json"

	"cleanpoint/internal/domain"
)

func decodeFeatureJSON(s string) map[string]float64 {
	m := map[string]float64{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &m)
	}
	return m
}

func decodeContribJSON(s string) []domain.SignalContribution {
	if s == "" {
		return nil
	}
	var out []domain.SignalContribution
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func decodeStringListJSON(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// EncodeFeatureJSON / EncodeContribJSON / EncodeStringListJSON are the
// canonical encoders used by producers.
func EncodeFeatureJSON(m map[string]float64) string {
	if m == nil {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func EncodeContribJSON(c []domain.SignalContribution) string {
	if c == nil {
		return "[]"
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func EncodeStringList(l []string) string {
	if l == nil {
		return "[]"
	}
	b, err := json.Marshal(l)
	if err != nil {
		return "[]"
	}
	return string(b)
}
