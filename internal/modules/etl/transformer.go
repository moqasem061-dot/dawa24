package etl

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ParseLegacyDatetime handles MySQL datetime formats and converts them explicitly to UTC.
func ParseLegacyDatetime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0000-00-00 00:00:00" || raw == "0000-00-00" {
		return nil, nil
	}

	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, raw, time.UTC); err == nil {
			utc := t.UTC()
			return &utc, nil
		}
	}

	return nil, fmt.Errorf("etl: unparseable legacy datetime %q", raw)
}

// ParseLegacyMoney converts legacy numeric/float columns into exact minor-unit money.Amount.
func ParseLegacyMoney(raw any) (money.Amount, error) {
	if raw == nil {
		return money.Zero, nil
	}
	switch v := raw.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return money.Zero, nil
		}
		return money.Parse(v)
	case float64:
		str := fmt.Sprintf("%.2f", v)
		return money.Parse(str)
	case int64:
		return money.FromMinor(v * 100), nil
	case int:
		return money.FromMinor(int64(v) * 100), nil
	default:
		return money.Zero, fmt.Errorf("etl: unknown money type %T", raw)
	}
}

// TransformToI18nText normalizes legacy string/JSON into bilingual i18n.Text.
func TransformToI18nText(raw string, defaultEn string) i18n.Text {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return i18n.New("", "")
	}

	// If raw is already JSON encoded
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			return i18n.New(m["ar"], m["en"])
		}
	}

	return i18n.New(raw, defaultEn)
}
