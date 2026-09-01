package files_test

import (
	"testing"

	"vps-dashboard-api/internal/files"
)

func TestSafePath(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "/"},
		{"/", "/"},
		{"/home/deploy", "/home/deploy"},
		{"/home/../etc/passwd", "/etc/passwd"},
		{"../../../etc/shadow", "/etc/shadow"},
		{"relative/path", "/relative/path"},
		{"/var/log/../log/nginx", "/var/log/nginx"},
		{"/..", "/"},
		{"/./test/./file", "/test/file"},
		{"/home/deploy/", "/home/deploy"}, // trailing slash removed
		{"/a/b/../../c", "/c"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := files.SafePath(tc.input)
			if got != tc.want {
				t.Errorf("SafePath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSafePathNoTraversal(t *testing.T) {
	// Ensure the result never starts with ".." or contains ".." that
	// would escape the root.
	cases := []string{
		"../../../../..",
		"/../../../../..",
		"/etc/../../..",
		"/a/b/c/../../../..",
	}
	for _, input := range cases {
		result := files.SafePath(input)
		if result == ".." || result == "../" {
			t.Errorf("SafePath(%q) escaped root: %q", input, result)
		}
		if len(result) >= 2 && result[:2] == ".." {
			t.Errorf("SafePath(%q) starts with ..: %q", input, result)
		}
	}
}
