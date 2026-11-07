package detector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ProjectType represents the classified application type.
type ProjectType string

const (
	ProjectDocker  ProjectType = "docker"
	ProjectNextJS  ProjectType = "nextjs"
	ProjectReact   ProjectType = "react"
	ProjectNodeJS  ProjectType = "nodejs"
	ProjectUnknown ProjectType = "unknown"
)

// PackageManager represents the detected package manager.
type PackageManager string

const (
	PkgManagerNPM  PackageManager = "npm"
	PkgManagerBun  PackageManager = "bun"
	PkgManagerPNPM PackageManager = "pnpm"
	PkgManagerYarn PackageManager = "yarn"
)

// ProjectSpec contains the detected project parameters and Dockerfile configuration.
type ProjectSpec struct {
	Type              ProjectType
	Framework         string // "vite", "cra", "nextjs", "nodejs", "docker"
	PackageManager    string // "npm", "bun", "pnpm", "yarn"
	OutputDir         string // "dist", "build", or custom outDir
	DefaultPort       int
	DockerfilePath    string
	GeneratedDocker   bool
	DockerfileContent string
}

type packageJSON struct {
	Name            string            `json:"name"`
	Main            string            `json:"main"`
	PackageManager  string            `json:"packageManager"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

var (
	viteOutDirRegex   = regexp.MustCompile(`outDir\s*:\s*['"` + "`" + `]([^'"` + "`" + `]+)['"` + "`" + `]`)
	craBuildPathRegex = regexp.MustCompile(`(?m)^\s*BUILD_PATH\s*=\s*(.+)`)
)

// Detect analyzes the project in sourceDir and returns a buildable ProjectSpec.
func Detect(sourceDir string, customDockerfile string) (*ProjectSpec, error) {
	// 1. Explicit custom Dockerfile
	if customDockerfile != "" {
		dfPath := filepath.Join(sourceDir, customDockerfile)
		if _, err := os.Stat(dfPath); err != nil {
			return nil, fmt.Errorf("specified Dockerfile '%s' not found in source directory: %w", customDockerfile, err)
		}
		return &ProjectSpec{
			Type:           ProjectDocker,
			Framework:      "docker",
			DefaultPort:    3000,
			DockerfilePath: customDockerfile,
		}, nil
	}

	// 2. Existing standard Dockerfile
	for _, name := range []string{"Dockerfile", "dockerfile"} {
		dfPath := filepath.Join(sourceDir, name)
		if _, err := os.Stat(dfPath); err == nil {
			return &ProjectSpec{
				Type:           ProjectDocker,
				Framework:      "docker",
				DefaultPort:    3000,
				DockerfilePath: name,
			}, nil
		}
	}

	// 3. Inspect package.json (Next.js, React, Node.js)
	pkgPath := filepath.Join(sourceDir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		var pkg packageJSON
		if err := json.Unmarshal(data, &pkg); err == nil {
			pkgMgr := detectPackageManager(sourceDir, pkg)

			// Check Next.js
			if hasDep(pkg, "next") {
				return &ProjectSpec{
					Type:              ProjectNextJS,
					Framework:         "nextjs",
					PackageManager:    string(pkgMgr),
					DefaultPort:       3000,
					DockerfilePath:    ".nugget.Dockerfile",
					GeneratedDocker:   true,
					DockerfileContent: nextjsDockerfile(sourceDir, pkgMgr),
				}, nil
			}

			// Check React / Vite
			if isReactOrVite(sourceDir, pkg) {
				isVite := isViteProject(sourceDir, pkg)
				isCRA := isCRAProject(pkg)
				outDir := detectReactOutputDir(sourceDir, isVite, isCRA)
				framework := "react"
				if isVite {
					framework = "vite"
				} else if isCRA {
					framework = "cra"
				}

				return &ProjectSpec{
					Type:              ProjectReact,
					Framework:         framework,
					PackageManager:    string(pkgMgr),
					OutputDir:         outDir,
					DefaultPort:       80,
					DockerfilePath:    ".nugget.Dockerfile",
					GeneratedDocker:   true,
					DockerfileContent: reactDockerfile(sourceDir, pkgMgr, outDir),
				}, nil
			}

			// Generic Node.js
			return &ProjectSpec{
				Type:              ProjectNodeJS,
				Framework:         "nodejs",
				PackageManager:    string(pkgMgr),
				DefaultPort:       3000,
				DockerfilePath:    ".nugget.Dockerfile",
				GeneratedDocker:   true,
				DockerfileContent: nodejsDockerfile(sourceDir, pkgMgr),
			}, nil
		}
	}

	return nil, fmt.Errorf("unsupported or ambiguous project structure: no Dockerfile or recognized Node/Next.js configuration found.\n" +
		"To deploy, add a Dockerfile to the repository or pass a custom path with --dockerfile")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasDep(pkg packageJSON, dep string) bool {
	if _, ok := pkg.Dependencies[dep]; ok {
		return true
	}
	if _, ok := pkg.DevDependencies[dep]; ok {
		return true
	}
	return false
}

func isReactOrVite(sourceDir string, pkg packageJSON) bool {
	if hasDep(pkg, "react") || hasDep(pkg, "react-dom") || hasDep(pkg, "react-scripts") {
		return true
	}
	if hasDep(pkg, "vite") || hasDep(pkg, "@vitejs/plugin-react") || hasDep(pkg, "@vitejs/plugin-react-swc") {
		return true
	}
	for _, name := range []string{"vite.config.ts", "vite.config.js", "vite.config.mjs", "vite.config.cjs"} {
		if fileExists(filepath.Join(sourceDir, name)) {
			return true
		}
	}
	for _, script := range pkg.Scripts {
		if strings.Contains(script, "vite") || strings.Contains(script, "react-scripts") {
			return true
		}
	}
	return false
}

func isViteProject(sourceDir string, pkg packageJSON) bool {
	if hasDep(pkg, "vite") || hasDep(pkg, "@vitejs/plugin-react") || hasDep(pkg, "@vitejs/plugin-react-swc") {
		return true
	}
	for _, name := range []string{"vite.config.ts", "vite.config.js", "vite.config.mjs", "vite.config.cjs"} {
		if fileExists(filepath.Join(sourceDir, name)) {
			return true
		}
	}
	for _, script := range pkg.Scripts {
		if strings.Contains(script, "vite") {
			return true
		}
	}
	return false
}

func isCRAProject(pkg packageJSON) bool {
	if hasDep(pkg, "react-scripts") {
		return true
	}
	for _, script := range pkg.Scripts {
		if strings.Contains(script, "react-scripts") {
			return true
		}
	}
	return false
}

func detectPackageManager(sourceDir string, pkg packageJSON) PackageManager {
	// 1. Check lockfiles first
	if fileExists(filepath.Join(sourceDir, "bun.lock")) || fileExists(filepath.Join(sourceDir, "bun.lockb")) {
		return PkgManagerBun
	}
	if fileExists(filepath.Join(sourceDir, "pnpm-lock.yaml")) {
		return PkgManagerPNPM
	}
	if fileExists(filepath.Join(sourceDir, "yarn.lock")) {
		return PkgManagerYarn
	}
	if fileExists(filepath.Join(sourceDir, "package-lock.json")) {
		return PkgManagerNPM
	}

	// 2. Check packageManager property in package.json
	if pkg.PackageManager != "" {
		pm := strings.ToLower(pkg.PackageManager)
		if strings.HasPrefix(pm, "bun") {
			return PkgManagerBun
		}
		if strings.HasPrefix(pm, "pnpm") {
			return PkgManagerPNPM
		}
		if strings.HasPrefix(pm, "yarn") {
			return PkgManagerYarn
		}
		if strings.HasPrefix(pm, "npm") {
			return PkgManagerNPM
		}
	}

	return PkgManagerNPM
}

func detectReactOutputDir(sourceDir string, isVite bool, isCRA bool) string {
	if isVite {
		// Look for custom outDir in vite.config.*
		for _, name := range []string{"vite.config.ts", "vite.config.js", "vite.config.mjs", "vite.config.cjs", "vite.config.json"} {
			cfgPath := filepath.Join(sourceDir, name)
			if data, err := os.ReadFile(cfgPath); err == nil {
				if match := viteOutDirRegex.FindSubmatch(data); len(match) > 1 {
					dir := strings.TrimSpace(string(match[1]))
					dir = strings.TrimPrefix(dir, "./")
					dir = strings.TrimPrefix(dir, "/")
					clean := filepath.Clean(dir)
					if clean != "" && clean != "." {
						return clean
					}
				}
			}
		}
		// Standard Vite output directory is dist
		return "dist"
	}

	if isCRA {
		// Check for BUILD_PATH in .env files
		for _, envName := range []string{".env", ".env.production", ".env.local"} {
			envPath := filepath.Join(sourceDir, envName)
			if data, err := os.ReadFile(envPath); err == nil {
				if match := craBuildPathRegex.FindSubmatch(data); len(match) > 1 {
					dir := strings.TrimSpace(string(match[1]))
					dir = strings.Trim(dir, `"'`)
					dir = strings.TrimPrefix(dir, "./")
					dir = strings.TrimPrefix(dir, "/")
					clean := filepath.Clean(dir)
					if clean != "" && clean != "." {
						return clean
					}
				}
			}
		}
		// Standard Create React App output directory is build
		return "build"
	}

	return "dist"
}

func reactDockerfile(sourceDir string, pkgMgr PackageManager, outDir string) string {
	var builderImage string
	var preInstall string
	var copyPkg string
	var installCmd string
	var buildCmd string

	switch pkgMgr {
	case PkgManagerBun:
		builderImage = "oven/bun:1-alpine"
		preInstall = "RUN apk add --no-cache nodejs\n"
		hasBunLock := fileExists(filepath.Join(sourceDir, "bun.lock"))
		hasBunLockb := fileExists(filepath.Join(sourceDir, "bun.lockb"))
		if hasBunLock && hasBunLockb {
			copyPkg = "COPY package.json bun.lock* ./"
		} else if hasBunLock {
			copyPkg = "COPY package.json bun.lock ./"
		} else if hasBunLockb {
			copyPkg = "COPY package.json bun.lockb ./"
		} else {
			copyPkg = "COPY package.json ./"
		}
		installCmd = "RUN bun install"
		buildCmd = "RUN bun run build"

	case PkgManagerPNPM:
		builderImage = "node:20-alpine"
		preInstall = "RUN corepack enable || npm install -g pnpm\n"
		if fileExists(filepath.Join(sourceDir, "pnpm-lock.yaml")) {
			copyPkg = "COPY package.json pnpm-lock.yaml ./"
		} else {
			copyPkg = "COPY package.json ./"
		}
		installCmd = "RUN pnpm install"
		buildCmd = "RUN pnpm run build"

	case PkgManagerYarn:
		builderImage = "node:20-alpine"
		if fileExists(filepath.Join(sourceDir, "yarn.lock")) {
			copyPkg = "COPY package.json yarn.lock ./"
		} else {
			copyPkg = "COPY package.json ./"
		}
		installCmd = "RUN yarn install"
		buildCmd = "RUN yarn run build"

	default: // npm
		builderImage = "node:20-alpine"
		if fileExists(filepath.Join(sourceDir, "package-lock.json")) {
			copyPkg = "COPY package*.json ./"
		} else {
			copyPkg = "COPY package.json ./"
		}
		installCmd = "RUN npm install"
		buildCmd = "RUN npm run build"
	}

	var nginxConfigStep string
	if fileExists(filepath.Join(sourceDir, "nginx.conf")) {
		nginxConfigStep = "COPY nginx.conf /etc/nginx/conf.d/default.conf\n"
	} else {
		nginxConfigStep = "RUN printf 'server {\\n" +
			"    listen 80;\\n" +
			"    server_name localhost;\\n" +
			"    location / {\\n" +
			"        root /usr/share/nginx/html;\\n" +
			"        index index.html index.htm;\\n" +
			"        try_files $uri $uri/ /index.html;\\n" +
			"    }\\n" +
			"    error_page 500 502 503 504 /50x.html;\\n" +
			"    location = /50x.html {\\n" +
			"        root /usr/share/nginx/html;\\n" +
			"    }\\n" +
			"}\\n' > /etc/nginx/conf.d/default.conf\n"
	}

	return fmt.Sprintf(`FROM %s AS builder
WORKDIR /app
%s%s
%s
COPY . .
%s

FROM nginx:alpine AS runner
%sCOPY --from=builder /app/%s /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`, builderImage, preInstall, copyPkg, installCmd, buildCmd, nginxConfigStep, outDir)
}

func nextjsDockerfile(sourceDir string, pkgMgr PackageManager) string {
	var builderImage string
	var copyPkg string
	var installCmd string
	var buildCmd string
	var runnerStartCmd string

	switch pkgMgr {
	case PkgManagerBun:
		builderImage = "oven/bun:1-alpine"
		hasBunLock := fileExists(filepath.Join(sourceDir, "bun.lock"))
		hasBunLockb := fileExists(filepath.Join(sourceDir, "bun.lockb"))
		if hasBunLock && hasBunLockb {
			copyPkg = "COPY package.json bun.lock* ./"
		} else if hasBunLock {
			copyPkg = "COPY package.json bun.lock ./"
		} else if hasBunLockb {
			copyPkg = "COPY package.json bun.lockb ./"
		} else {
			copyPkg = "COPY package.json ./"
		}
		installCmd = "RUN bun install"
		buildCmd = "RUN bun run build"
		runnerStartCmd = `["bun", "run", "start"]`

	case PkgManagerPNPM:
		builderImage = "node:20-alpine"
		if fileExists(filepath.Join(sourceDir, "pnpm-lock.yaml")) {
			copyPkg = "COPY package.json pnpm-lock.yaml ./"
		} else {
			copyPkg = "COPY package.json ./"
		}
		installCmd = "RUN corepack enable || npm install -g pnpm && pnpm install"
		buildCmd = "RUN pnpm run build"
		runnerStartCmd = `["npm", "run", "start"]`

	case PkgManagerYarn:
		builderImage = "node:20-alpine"
		if fileExists(filepath.Join(sourceDir, "yarn.lock")) {
			copyPkg = "COPY package.json yarn.lock ./"
		} else {
			copyPkg = "COPY package.json ./"
		}
		installCmd = "RUN yarn install"
		buildCmd = "RUN yarn run build"
		runnerStartCmd = `["yarn", "start"]`

	default: // npm
		builderImage = "node:20-alpine"
		if fileExists(filepath.Join(sourceDir, "package-lock.json")) {
			copyPkg = "COPY package*.json ./"
		} else {
			copyPkg = "COPY package.json ./"
		}
		installCmd = "RUN npm install"
		buildCmd = "RUN npm run build"
		runnerStartCmd = `["npm", "run", "start"]`
	}

	return fmt.Sprintf(`FROM %s AS builder
WORKDIR /app
%s
%s
COPY . .
%s

FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
COPY --from=builder /app/package*.json ./
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/public ./public
EXPOSE 3000
CMD %s
`, builderImage, copyPkg, installCmd, buildCmd, runnerStartCmd)
}

func nodejsDockerfile(sourceDir string, pkgMgr PackageManager) string {
	switch pkgMgr {
	case PkgManagerBun:
		hasBunLock := fileExists(filepath.Join(sourceDir, "bun.lock"))
		hasBunLockb := fileExists(filepath.Join(sourceDir, "bun.lockb"))
		var copyPkg string
		if hasBunLock && hasBunLockb {
			copyPkg = "COPY package.json bun.lock* ./"
		} else if hasBunLock {
			copyPkg = "COPY package.json bun.lock ./"
		} else if hasBunLockb {
			copyPkg = "COPY package.json bun.lockb ./"
		} else {
			copyPkg = "COPY package.json ./"
		}
		return fmt.Sprintf(`FROM oven/bun:1-alpine
WORKDIR /app
%s
RUN bun install
COPY . .
EXPOSE 3000
CMD ["bun", "run", "start"]
`, copyPkg)

	case PkgManagerPNPM:
		return `FROM node:20-alpine
WORKDIR /app
COPY package.json pnpm-lock.yaml* ./
RUN corepack enable || npm install -g pnpm && pnpm install
COPY . .
EXPOSE 3000
CMD ["pnpm", "start"]
`
	case PkgManagerYarn:
		return `FROM node:20-alpine
WORKDIR /app
COPY package.json yarn.lock* ./
RUN yarn install
COPY . .
EXPOSE 3000
CMD ["yarn", "start"]
`
	default:
		return `FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "start"]
`
	}
}
