package repository_test

import (
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// Missing browser path folding must not let a proxy reference escape the
// binding policy or disappear from historical GC reference checks.
func TestNormalizeAvatarURLPath(t *testing.T) {
	const key = "avatars/a.png"
	for _, tt := range []struct {
		name, value, want string
		internal          bool
	}{
		{"empty", "", "", false},
		{"raw", key, key, true},
		{"relative", "/api/v1/files/avatars/a.png", key, true},
		{"absolute", "https://app.example.invalid/api/v1/files/avatars/a.png?v=1#photo", key, true},
		{"dot_before_route", "https://app.example.invalid/x/../api/v1/files/avatars/a.png", key, true},
		{"relative_dot", "/api/./v1/files/avatars/a.png", key, true},
		{"dot_in_key", "https://app.example.invalid/api/v1/files/avatars/x/../a.png", key, true},
		{"escaped_dot", "https://app.example.invalid/x/%2e%2E/api/v1/files/avatars/a.png", key, true},
		{"mixed_dot", "/api/v1/files/avatars/x/.%2e/a.png", key, true},
		{"escaped_namespace", "/api/v1/files/%61vatars/a.png", key, true},
		{"escaped_separator", "/api/v1/files/avatars%2Fa.png", key, true},
		{"https_backslash_path", `https://app.example.invalid\api\v1\files\avatars\a.png`, key, true},
		{"http_backslash_authority", `http:\\app.example.invalid\api\v1\files\avatars\a.png`, key, true},
		{"extra_authority_slashes", "https:////app.example.invalid/api/v1/files/avatars/a.png", key, true},
		{"network_authority_slashes", "///app.example.invalid/api/v1/files/avatars/a.png", key, true},
		{"authority_not_path", "https://///api/v1/files/avatars/a.png", "", false},
		{"network_authority_not_path", "///api/v1/files/avatars/a.png", "", true},
		{"uppercase_scheme", "HTTPS://app.example.invalid/api/v1/files/avatars/a.png", key, true},
		{"relative_backslash", `/api\v1/files\avatars/a.png`, key, true},
		{"browser_whitespace", " \thttps://app.example.invalid/api/v1/fi\nles/avatars/a.png\r ", key, true},
		{"empty_segment_before_dot", "https://app.example.invalid/api/v1/files/avatars/x//../a.png", "avatars/x/a.png", true},
		{"scheme_text_in_path", "/api/v1/files/avatars/http:////../../a.png", "", true},
		{"query", "https://external.example.invalid/photo.png?next=/api/v1/files/avatars/a.png", "", false},
		{"fragment", "https://external.example.invalid/photo.png#/api/v1/files/avatars/a.png", "", false},
		{"ordinary_external", "https://external.example.invalid/avatars/a.png", "", false},
		{"leaves_namespace", "https://app.example.invalid/api/v1/files/avatars/../photo.png", "", false},
		{"leaves_route", "https://app.example.invalid/api/v1/files/avatars/../../photo.png", "", false},
		// nginx proxy_pass has no URI: normalized location matching does not
		// rewrite the original URI forwarded to Gin. Do not merge these slashes.
		{"repeated_route_slash", "https://app.example.invalid/api//v1/files/avatars/a.png", "", false},
		{"repeated_key_slash", "https://app.example.invalid/api/v1/files/avatars//a.png", "", true},
		{"trailing_slash", "https://app.example.invalid/api/v1/files/avatars/a.png/", "", true},
		{"terminal_dot", "https://app.example.invalid/api/v1/files/avatars/a.png/.", "", true},
		{"encoded_slash_not_browser_dot", "https://app.example.invalid/x%2F..%2Fapi/v1/files/avatars/a.png", "", false},
		{"encoded_backslash_not_separator", "https://app.example.invalid/api/v1/files/avatars%5Ca.png", "", false},
		{"no_recursive_decode", "https://app.example.invalid/api/v1/files/%2561vatars/a.png", "", false},
		{"escaped_query_is_key", "/api/v1/files/avatars/a%3Fx.png?ignored=1", "avatars/a?x.png", true},
		{"escaped_fragment_is_key", "/api/v1/files/avatars/a%23x.png#ignored", "avatars/a#x.png", true},
		{"raw_dot_unchanged", "avatars/x/../a.png", "", true},
		{"raw_slashes_unchanged", "avatars//a.png", "", true},
		{"raw_backslash_unchanged", `avatars\a.png`, "", true},
		{"raw_escape_unchanged", "avatars%2Fa.png", "", true},
		{"raw_whitespace_unchanged", " avatars/a.png ", "", true},
		{"raw_dot_prefix_unchanged", ".//api/v1/files/avatars/a.png", "", true},
		{"raw_escaped_dot_prefix_unchanged", "%2e//api/v1/files/avatars/a.png", "", true},
		{"invalid_escape", "/api/v1/files/avatars/%zz.png", "", true},
		{"nul_key", "/api/v1/files/avatars/a%00.png", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == "" {
				want = tt.value
			}
			got, internal := repository.NormalizeAvatar(tt.value)
			if got != want || internal != tt.internal {
				t.Fatalf("NormalizeAvatar(%q) = (%q, %v), want (%q, %v)", tt.value, got, internal, want, tt.internal)
			}
		})
	}
}
