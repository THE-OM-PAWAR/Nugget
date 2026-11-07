package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/nugget-sh/nugget/agent/internal/doctor"
)

// RunDoctor performs host, container, configuration, and proxy diagnostic checks.
func RunDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	configDir := fs.String("config-dir", "", "Path to Nugget configuration directory")
	dataDir := fs.String("data-dir", "", "Path to Nugget data directory")
	jsonOutput := fs.Bool("json", false, "Output results in JSON format")

	if err := fs.Parse(args); err != nil {
		return err
	}

	report := doctor.RunDiagnostics(*configDir, *dataDir)

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	printDoctorReport(report)

	if report.FailCount > 0 {
		return fmt.Errorf("diagnostics completed with %d critical failure(s)", report.FailCount)
	}

	return nil
}

func printDoctorReport(report *doctor.DoctorReport) {
	fmt.Println("================================================================================")
	fmt.Println("                       Nugget System Health & Diagnostics                       ")
	fmt.Println("================================================================================")

	var currentCategory string
	for _, res := range report.Results {
		if res.Category != currentCategory {
			currentCategory = res.Category
			fmt.Printf("\n[%s]\n", currentCategory)
		}

		switch res.Level {
		case doctor.LevelPass:
			fmt.Printf("  \033[32m[✓]\033[0m %-28s %s\n", res.Name, res.Message)
		case doctor.LevelWarn:
			fmt.Printf("  \033[33m[!]\033[0m %-28s %s\n", res.Name, res.Message)
			if res.Advice != "" {
				fmt.Printf("      \033[2mAdvice: %s\033[0m\n", res.Advice)
			}
		case doctor.LevelFail:
			fmt.Printf("  \033[31m[✗]\033[0m %-28s %s\n", res.Name, res.Message)
			if res.Advice != "" {
				fmt.Printf("      \033[1;31mAdvice: %s\033[0m\n", res.Advice)
			}
		}
	}

	fmt.Println("\n--------------------------------------------------------------------------------")
	summaryColor := "\033[32m"
	if report.FailCount > 0 {
		summaryColor = "\033[31m"
	} else if report.WarnCount > 0 {
		summaryColor = "\033[33m"
	}

	fmt.Printf("%sSummary: %d passed, %d warnings, %d failed\033[0m\n",
		summaryColor, report.PassCount, report.WarnCount, report.FailCount)

	if report.FailCount == 0 && report.WarnCount == 0 {
		fmt.Println("✓ All checks passed! Nugget is fully operational and production-ready.")
	} else if report.FailCount == 0 {
		fmt.Println("! Host is operational with minor warnings. Review recommendations above.")
	} else {
		fmt.Println("✗ Critical issues detected that may prevent deployments or agent stability.")
	}
	fmt.Println("================================================================================")
}

