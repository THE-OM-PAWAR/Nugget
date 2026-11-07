package detector

import (
	"os"
	"path/filepath"
	"strings"
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

func TestDetectReact_Vite_Npm(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "vite-react-app",
		"scripts": {
			"dev": "vite",
			"build": "vite build"
		},
		"devDependencies": {
			"vite": "^5.0.0",
			"@vitejs/plugin-react": "^4.2.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}
	// Simulate package-lock.json
	if err := os.WriteFile(filepath.Join(tmp, "package-lock.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := Detect(tmp, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if spec.Type != ProjectReact {
		t.Fatalf("Expected ProjectReact, got %s", spec.Type)
	}
	if spec.Framework != "vite" {
		t.Errorf("Expected Framework 'vite', got %s", spec.Framework)
	}
	if spec.PackageManager != "npm" {
		t.Errorf("Expected PackageManager 'npm', got %s", spec.PackageManager)
	}
	if spec.OutputDir != "dist" {
		t.Errorf("Expected OutputDir 'dist', got %s", spec.OutputDir)
	}
	if spec.DefaultPort != 80 {
		t.Errorf("Expected DefaultPort 80 for Nginx, got %d", spec.DefaultPort)
	}

	// Verify Dockerfile contents
	content := spec.DockerfileContent
	if !strings.Contains(content, "FROM node:20-alpine AS builder") {
		t.Errorf("Expected node:20-alpine builder for npm, got:\n%s", content)
	}
	if !strings.Contains(content, "RUN npm install") {
		t.Errorf("Expected npm install, got:\n%s", content)
	}
	if !strings.Contains(content, "RUN npm run build") {
		t.Errorf("Expected npm run build, got:\n%s", content)
	}
	if !strings.Contains(content, "FROM nginx:alpine AS runner") {
		t.Errorf("Expected nginx:alpine runner, got:\n%s", content)
	}
	if !strings.Contains(content, "COPY --from=builder /app/dist /usr/share/nginx/html") {
		t.Errorf("Expected COPY from /app/dist, got:\n%s", content)
	}
	if !strings.Contains(content, "EXPOSE 80") {
		t.Errorf("Expected EXPOSE 80, got:\n%s", content)
	}
	if !strings.Contains(content, "try_files $uri $uri/ /index.html;") {
		t.Errorf("Expected SPA routing in Nginx config, got:\n%s", content)
	}
}

func TestDetectReact_Vite_Bun(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "vite-bun-app",
		"devDependencies": {
			"vite": "^5.0.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}
	// Create bun.lock
	if err := os.WriteFile(filepath.Join(tmp, "bun.lock"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := Detect(tmp, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if spec.PackageManager != "bun" {
		t.Errorf("Expected PackageManager 'bun', got %s", spec.PackageManager)
	}

	content := spec.DockerfileContent
	if !strings.Contains(content, "FROM oven/bun:1-alpine AS builder") {
		t.Errorf("Expected oven/bun:1-alpine builder, got:\n%s", content)
	}
	if !strings.Contains(content, "RUN bun install") {
		t.Errorf("Expected bun install, got:\n%s", content)
	}
	if !strings.Contains(content, "RUN bun run build") {
		t.Errorf("Expected bun run build, got:\n%s", content)
	}
	if !strings.Contains(content, "COPY --from=builder /app/dist /usr/share/nginx/html") {
		t.Errorf("Expected COPY from /app/dist, got:\n%s", content)
	}
}

func TestDetectReact_Vite_CustomOutputDir(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "vite-custom-dist",
		"devDependencies": {
			"vite": "^5.0.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}
	viteConfig := `import { defineConfig } from 'vite';
export default defineConfig({
  build: {
    outDir: 'my-custom-output',
  },
});`
	if err := os.WriteFile(filepath.Join(tmp, "vite.config.ts"), []byte(viteConfig), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := Detect(tmp, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if spec.OutputDir != "my-custom-output" {
		t.Errorf("Expected OutputDir 'my-custom-output', got %s", spec.OutputDir)
	}

	content := spec.DockerfileContent
	if !strings.Contains(content, "COPY --from=builder /app/my-custom-output /usr/share/nginx/html") {
		t.Errorf("Expected COPY from /app/my-custom-output, got:\n%s", content)
	}
	if strings.Contains(content, "/app/dist") || strings.Contains(content, "/app/build") {
		t.Errorf("Dockerfile should not contain /app/dist or /app/build when custom outDir is specified, got:\n%s", content)
	}
}

func TestDetectReact_CRA_Npm(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "cra-app",
		"dependencies": {
			"react": "^18.2.0",
			"react-dom": "^18.2.0",
			"react-scripts": "5.0.1"
		}
	}`
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := Detect(tmp, "")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if spec.Framework != "cra" {
		t.Errorf("Expected Framework 'cra', got %s", spec.Framework)
	}
	if spec.OutputDir != "build" {
		t.Errorf("Expected OutputDir 'build' for CRA, got %s", spec.OutputDir)
	}

	content := spec.DockerfileContent
	if !strings.Contains(content, "COPY --from=builder /app/build /usr/share/nginx/html") {
		t.Errorf("Expected COPY from /app/build for CRA, got:\n%s", content)
	}
}

func TestDetectReact_ExistingDockerfile(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "react-with-dockerfile",
		"devDependencies": {
			"vite": "^5.0.0"
		}
	}`
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "Dockerfile"), []byte("FROM alpine\n"), 0644); err != nil {
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
		t.Errorf("Expected GeneratedDocker false when Dockerfile already exists")
	}
}

func TestDetectReact_SPARoutingInNginx(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "spa-routing-test",
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

	content := spec.DockerfileContent
	// Must configure Nginx default.conf with try_files fallback
	if !strings.Contains(content, "try_files $uri $uri/ /index.html;") {
		t.Errorf("Missing try_files fallback for React SPA in generated Dockerfile:\n%s", content)
	}
}

func TestDetectReact_Regression_NoAppBuildInVite(t *testing.T) {
	tmp := t.TempDir()
	pkgJSON := `{
		"name": "vite-regression-test",
		"devDependencies": {
			"@vitejs/plugin-react": "^4.0.0",
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

	content := spec.DockerfileContent
	// REGRESSION CHECK: Ensure /app/build is NEVER referenced in Vite Dockerfiles!
	if strings.Contains(content, "/app/build ") || strings.Contains(content, "/app/build\n") || strings.Contains(content, "/app/build ./") {
		t.Errorf("REGRESSION BUG DETECTED: Vite Dockerfile contains reference to /app/build:\n%s", content)
	}
	if !strings.Contains(content, "/app/dist") {
		t.Errorf("Expected Vite Dockerfile to reference /app/dist, got:\n%s", content)
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
	_, err := Detect(tmp, "")
	if err == nil {
		t.Fatalf("Expected error for unsupported project, got nil")
	}
}

func TestDetectOmpawarWorkspace(t *testing.T) {
	ompawarPath := "/Users/om/@code26/ompawar"
	if _, err := os.Stat(ompawarPath); err != nil {
		t.Skip("ompawar workspace not found, skipping local path check")
	}

	spec, err := Detect(ompawarPath, "")
	if err != nil {
		t.Fatalf("Detect on ompawar failed: %v", err)
	}

	if spec.Type != ProjectReact {
		t.Errorf("Expected ProjectReact for ompawar, got %s", spec.Type)
	}
	if spec.Framework != "vite" {
		t.Errorf("Expected Framework 'vite' for ompawar, got %s", spec.Framework)
	}
	if spec.PackageManager != "bun" {
		t.Errorf("Expected PackageManager 'bun' for ompawar, got %s", spec.PackageManager)
	}
	if spec.OutputDir != "dist" {
		t.Errorf("Expected OutputDir 'dist' for ompawar, got %s", spec.OutputDir)
	}

	// Verify no /app/build reference
	if strings.Contains(spec.DockerfileContent, "/app/build") {
		t.Errorf("ompawar Dockerfile must not contain /app/build:\n%s", spec.DockerfileContent)
	}
	if !strings.Contains(spec.DockerfileContent, "COPY --from=builder /app/dist /usr/share/nginx/html") {
		t.Errorf("ompawar Dockerfile must copy from /app/dist to nginx html:\n%s", spec.DockerfileContent)
	}
}
