package git

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	// githubURLRegex matches standard GitHub repository URLs.
	githubURLRegex = regexp.MustCompile(`^(?:https?://github\.com/|git@github\.com:)([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+?)(?:\.git)?/?$`)
	// branchRegex validates safe branch names.
	branchRegex = regexp.MustCompile(`^[a-zA-Z0-9/_.-]+$`)
)

// ValidateURL checks whether rawURL is a valid, safe GitHub repository URL.
func ValidateURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return errors.New("repository URL cannot be empty")
	}

	// Reject any strings starting with a dash to prevent git argument injection
	if strings.HasPrefix(trimmed, "-") {
		return fmt.Errorf("invalid repository URL: starts with illegal character '-'")
	}

	if !githubURLRegex.MatchString(trimmed) {
		return fmt.Errorf("invalid GitHub repository URL '%s': must match https://github.com/owner/repo or git@github.com:owner/repo.git", trimmed)
	}

	// Extra check for HTTP/HTTPS URLs
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return fmt.Errorf("malformed repository URL: %w", err)
		}
		if parsed.Host != "github.com" {
			return fmt.Errorf("only github.com repositories are supported, got '%s'", parsed.Host)
		}
	}

	return nil
}

// ParseRepoName extracts the repository name from a GitHub URL (e.g. "https://github.com/user/my-app" -> "my-app").
func ParseRepoName(rawURL string) string {
	matches := githubURLRegex.FindStringSubmatch(strings.TrimSpace(rawURL))
	if len(matches) >= 3 {
		name := matches[2]
		return strings.TrimSuffix(name, ".git")
	}
	return "app"
}

// Clone clones a public GitHub repository into targetDir.
// If targetDir already exists, it can be cleaned or updated.
// Uses a timeout and shallow clone (--depth 1) for speed and safety.
func Clone(ctx context.Context, repoURL, branch, targetDir string) error {
	if err := ValidateURL(repoURL); err != nil {
		return err
	}

	if branch != "" && !branchRegex.MatchString(branch) {
		return fmt.Errorf("invalid branch name '%s'", branch)
	}

	// Ensure git CLI is available
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is not installed or not in PATH: %w", err)
	}

	// Prepare clean target directory
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("failed to clean target directory %s: %w", targetDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory for %s: %w", targetDir, err)
	}

	// Build safe git clone command arguments
	args := []string{"clone", "--depth", "1"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, repoURL, targetDir)

	cloneCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cloneCtx, gitPath, args...)
	// Prevent interactive credentials prompts from hanging the process
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=echo",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		if cloneCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git clone timed out after 3 minutes")
		}
		if strings.Contains(outStr, "Repository not found") || strings.Contains(outStr, "Authentication failed") {
			return fmt.Errorf("failed to clone repository: repository not found or is private (Nugget currently supports public repositories)")
		}
		if strings.Contains(outStr, "Remote branch") && strings.Contains(outStr, "not found") {
			return fmt.Errorf("branch '%s' not found on remote repository", branch)
		}
		return fmt.Errorf("git clone failed: %s (%w)", outStr, err)
	}

	return nil
}

