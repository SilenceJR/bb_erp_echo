package middleware

import "testing"

func TestPermissionReadFallbacksAreLimitedToManagementLists(t *testing.T) {
	tests := []struct {
		object string
		want   []permissionFallback
	}{
		{"/api/v1/system/users", []permissionFallback{{object: "/api/v1/system/users", action: "write"}}},
		{"/api/v1/system/roles", []permissionFallback{{object: "/api/v1/system/roles", action: "write"}, {object: "/api/v1/system/users", action: "write"}}},
		{"/api/v1/system/permissions", []permissionFallback{{object: "/api/v1/system/roles", action: "write"}}},
	}
	for _, tt := range tests {
		got := permissionReadFallbacks(tt.object, "read")
		if len(got) != len(tt.want) {
			t.Fatalf("fallbacks for %s = %#v, want %#v", tt.object, got, tt.want)
		}
		for index := range got {
			if got[index] != tt.want[index] {
				t.Fatalf("fallback %d for %s = %#v, want %#v", index, tt.object, got[index], tt.want[index])
			}
		}
	}
	if got := permissionReadFallbacks("/api/v1/customers", "read"); len(got) != 0 {
		t.Fatalf("business read unexpectedly inherited write: %#v", got)
	}
	if got := permissionReadFallbacks("/api/v1/system/users", "write"); len(got) != 0 {
		t.Fatalf("write action unexpectedly received fallback: %#v", got)
	}
}
