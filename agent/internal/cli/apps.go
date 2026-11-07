package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nugget-sh/nugget/agent/internal/config"
	"github.com/nugget-sh/nugget/agent/internal/docker"
	"github.com/nugget-sh/nugget/agent/internal/health"
	"github.com/nugget-sh/nugget/agent/internal/store"
)

// AppSummary represents formatted application status for JSON or tabular display.
type AppSummary struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	Status      string `json:"status"`
	InternalURL string `json:"internal_url"`
	PublicURL   string `json:"public_url"`
	ContainerID string `json:"container_id"`
	RepoURL     string `json:"repo_url"`
}

// RunApps executes the `nugget apps` command to list deployed applications.
func RunApps(args []string) error {
	fs := flag.NewFlagSet("apps", flag.ContinueOnError)
	configDirFlag := fs.String("config-dir", "", "Custom configuration directory")
	dataDirFlag := fs.String("data-dir", "", "Custom data directory")
	jsonFlag := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		return err
	}

	configDir := *configDirFlag
	if configDir == "" {
		configDir = config.DefaultConfigDir()
	}

	dataDir := *dataDirFlag
	if dataDir == "" {
		dataDir = config.DefaultDataDir(configDir)
	}

	st := store.NewStore(dataDir)
	apps, err := st.ListApplications()
	if err != nil {
		return fmt.Errorf("failed to list applications: %w", err)
	}

	dClient, _ := docker.NewClient()
	ctx := context.Background()

	var summaries []AppSummary
	for _, app := range apps {
		appHealth := health.CheckAppHealth(ctx, dClient, app.ActiveContainerID, app.Port)
		statusStr := string(appHealth.Status)
		if app.Status == "stopped" {
			statusStr = "stopped"
		}

		summaries = append(summaries, AppSummary{
			Name:        app.Name,
			ID:          app.ID,
			Status:      statusStr,
			InternalURL: fmt.Sprintf("http://localhost:%d", app.Port),
			PublicURL:   app.PublicURL,
			ContainerID: app.ActiveContainerID,
			RepoURL:     app.RepoURL,
		})
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(summaries, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(summaries) == 0 {
		fmt.Println("No applications deployed yet.")
		fmt.Println("Run 'nugget deploy <github-repository-url>' to deploy an application.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tINTERNAL PORT\tPUBLIC URL\tCONTAINER")
	for _, s := range summaries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			s.Name,
			s.Status,
			s.InternalURL,
			s.PublicURL,
			s.ContainerID,
		)
	}
	return w.Flush()
}

