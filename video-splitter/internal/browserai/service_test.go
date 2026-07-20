package browserai

import (
	"testing"
)

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello_world"},
		{"test/path:name*?", "test_path_name__"},
		{"../../../path/traversal", "path_traversal"},
		{"a\\b:c*d?e\"f<g>h|i", "a_b_c_d_e_f_g_h_i"},
	}

	for _, tt := range tests {
		result := sanitizeFileName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeFileName(%q) = %q; expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestFindChromeExecutable(t *testing.T) {
	// Should not crash and find something or return error
	path, err := FindChromeExecutable("")
	if err != nil {
		t.Logf("Chrome not found on this machine: %v", err)
	} else {
		t.Logf("Found Chrome executable: %s", path)
	}
}
