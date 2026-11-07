package detector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectExistingDockerfile(t *testing.T) {
	tmp := t.TempDir()
	dfPath := filepath.Join(tmp, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte("FROM alpine\nCMD echo hi"), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := Detect(tmp, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if spec.Type != ProjectDocker {
		t.Errorf("Expected ProjectDocker, got %s", spec.Type)
	}
	if spec.GeneratedDocker {
		t.Errorf("Expected GeneratedDocker to be false")
	}
}

func TestDetectNextJS(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "my-next-app",
		"dependencies": {
			"next": "14.0.0",
			"react": "18.2.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := Detect(tmp, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if spec.Type != ProjectNextJS {
		t.Errorf("Expected ProjectNextJS, got %s", spec.Type)
	}
	if !spec.GeneratedDocker {
		t.Errorf("Expected GeneratedDocker to be true")
	}
}

func TestDetectReact(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "my-react-app",
		"devDependencies": {
			"vite": "^5.0.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := Detect(tmp, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if spec.Type != ProjectReact {
		t.Errorf("Expected ProjectReact, got %s", spec.Type)
	}
}

func TestDetectGenericNode(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "express-api",
		"dependencies": {
			"express": "^4.18.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := Detect(tmp, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if spec.Type != ProjectNodeJS {
		t.Errorf("Expected ProjectNodeJS, got %s", spec.Type)
	}
}

func TestDetectUnsupported(t *testing.T) {
	tmp := t.TempDir()
	// Empty folder
	_, err := Detect(tmp, "")
	if err == nil {
		t.Fatalf("Expected error for unsupported project, got nil")
	}
}

