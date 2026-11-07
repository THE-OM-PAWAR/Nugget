package detector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// ProjectSpec contains the detected project parameters and Dockerfile configuration.
type ProjectSpec struct {
	Type              ProjectType
	DefaultPort       int
	DockerfilePath    string
	GeneratedDocker   bool
	DockerfileContent string
}

type packageJSON struct {
	Name            string            `json:"name"`
	Main            string            `json:"main"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

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
			// Check Next.js
			if hasDep(pkg, "next") {
				return &ProjectSpec{
					Type:              ProjectNextJS,
					DefaultPort:       3000,
					DockerfilePath:    ".nugget.Dockerfile",
					GeneratedDocker:   true,
					DockerfileContent: nextjsDockerfile(),
				}, nil
			}

			// Check React / Vite
			if hasDep(pkg, "react-scripts") || hasDep(pkg, "vite") || hasDep(pkg, "@vitejs/plugin-react") {
				return &ProjectSpec{
					Type:              ProjectReact,
					DefaultPort:       3000,
					DockerfilePath:    ".nugget.Dockerfile",
					GeneratedDocker:   true,
					DockerfileContent: reactDockerfile(),
				}, nil
			}

			// Generic Node.js
			return &ProjectSpec{
				Type:              ProjectNodeJS,
				DefaultPort:       3000,
				DockerfilePath:    ".nugget.Dockerfile",
				GeneratedDocker:   true,
				DockerfileContent: nodejsDockerfile(),
			}, nil
		}
	}

	return nil, fmt.Errorf("unsupported or ambiguous project structure: no Dockerfile or recognized Node/Next.js configuration found.\n" +
		"To deploy, add a Dockerfile to the repository or pass a custom path with --dockerfile")
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

func nextjsDockerfile() string {
	return `FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
COPY --from=builder /app/package*.json ./
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/public ./public
EXPOSE 3000
CMD ["npm", "run", "start"]
`
}

func reactDockerfile() string {
	return `FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
RUN npm install -g serve
COPY --from=builder /app/build ./build
COPY --from=builder /app/dist ./dist
EXPOSE 3000
CMD ["sh", "-c", "if [ -d ./dist ]; then serve -s dist -l 3000; else serve -s build -l 3000; fi"]
`
}

func nodejsDockerfile() string {
	return `FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "start"]
`
}

