package platformadmin

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/reqcache"
)

// setting reads a system setting once per request. The gateway key, the AI
// configuration and the site settings were each read three or four times while
// one dashboard rendered. Every caller gets its own copy of the value, because
// several of them edit the map before storing it back.
func (s *Service) setting(ctx context.Context, key string) (*SystemSetting, error) {
	got, err := reqcache.Get(ctx, settingKey(key), func() (*SystemSetting, error) {
		return s.repo.GetSetting(ctx, key)
	})
	if err != nil || got == nil {
		return got, err
	}
	c := *got
	c.Value = cloneMap(got.Value)
	return &c, nil
}

// storeSetting writes a setting and drops its memoised read.
func (s *Service) storeSetting(ctx context.Context, setting *SystemSetting) error {
	defer reqcache.Forget(ctx, settingKey(setting.Key))
	return s.repo.SetSetting(ctx, setting)
}

func settingKey(key string) string { return "platform_admin.setting:" + key }

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneAny(v)
	}
	return out
}

func cloneAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneMap(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = cloneAny(e)
		}
		return out
	default:
		return v
	}
}
