package git

import (
	"testing"
)

func TestValidateURL(t *testing.T) {
	valid := []string{
		"https://github.com/facebook/react",
		"https://github.com/vercel/next.js.git",
		"http://github.com/golang/go",
		"git@github.com:nugget-sh/nugget.git",
		"https://github.com/my-org/my-project/",
	}

	for _, u := range valid {
		if err := ValidateURL(u); err != nil {
			t.Errorf("ValidateURL(%q) returned unexpected error: %v", u, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"--upload-pack=sh",
		"https://gitlab.com/owner/repo",
		"https://bitbucket.org/owner/repo",
		"https://github.com",
		"https://github.com/",
		"https://github.com/onlyowner",
		"ftp://github.com/owner/repo",
		"https://malicious.com/github.com/owner/repo",
	}

	for _, u := range invalid {
		if err := ValidateURL(u); err == nil {
			t.Errorf("ValidateURL(%q) expected error, got nil", u)
		}
	}
}

func TestParseRepoName(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://github.com/vercel/next.js", "next.js"},
		{"https://github.com/facebook/react.git", "react"},
		{"git@github.com:owner/my-service.git", "my-service"},
		{"https://github.com/user/simple-api/", "simple-api"},
	}

	for _, tt := range tests {
		actual := ParseRepoName(tt.url)
		if actual != tt.expected {
			t.Errorf("ParseRepoName(%q) = %q; want %q", tt.url, actual, tt.expected)
		}
	}
}

