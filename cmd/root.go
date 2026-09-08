package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/BRO3886/healthsync/internal/people"
	"github.com/BRO3886/healthsync/internal/skills"
	"github.com/BRO3886/healthsync/internal/storage"
	"github.com/BRO3886/healthsync/internal/update"
)

var (
	dbPath  string
	dataDir string
	person  string
)

// updateResultCh receives the background update check result (if any).
var updateResultCh = make(chan *update.Result, 1)

var rootCmd = &cobra.Command{
	Use:   "healthsync",
	Short: "Sync Apple Health export data into a local SQLite database",
	Long: `healthsync parses Apple Health export files (.zip or .xml) and stores
the data in a local SQLite database for easy querying.

Imports every record type in the export (100+ metrics, workouts with routes
and heart-rate zones, ECGs, activity rings, HRV beat-to-beat) and serves a
multi-person web dashboard with "healthsync server".`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if shouldCheckForUpdate(cmd) {
			go func() {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					updateResultCh <- nil
					return
				}
				updateResultCh <- update.Check(homeDir, Version)
			}()
		} else {
			updateResultCh <- nil
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		printUpdateNotice()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", storage.DefaultDBPath(), "path to SQLite database (single-user mode)")
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", people.DefaultDataDir(), "data directory holding people.db and per-person databases ($HEALTHSYNC_DATA_DIR)")
	rootCmd.PersistentFlags().StringVar(&person, "person", "", "operate on this person's database (id or name) inside --data-dir instead of --db")
}

// resolveDBPath returns the database to use: the --person database inside the
// data dir when --person is set, otherwise --db.
func resolveDBPath() (string, error) {
	if person == "" {
		return dbPath, nil
	}
	store, err := people.Open(dataDir)
	if err != nil {
		return "", fmt.Errorf("opening data dir: %w", err)
	}
	defer store.Close()
	p, err := store.Get(person)
	if err != nil {
		return "", fmt.Errorf("person %q: %w", person, err)
	}
	return store.DBPath(p.ID), nil
}

// shouldCheckForUpdate returns false for commands/contexts where the check should be skipped.
func shouldCheckForUpdate(cmd *cobra.Command) bool {
	if os.Getenv("HEALTHSYNC_NO_UPDATE_CHECK") != "" {
		return false
	}
	if Version == "" || Version == "dev" {
		return false
	}
	name := cmd.Name()
	if name == "version" || name == "completion" || name == "skills" {
		return false
	}
	// Skip if stdout is not a TTY (piped output)
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return true
}

// printUpdateNotice prints update and skills staleness notices to stderr.
func printUpdateNotice() {
	var result *update.Result
	select {
	case result = <-updateResultCh:
	default:
		result = nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	yellow := color.New(color.FgYellow)

	if result != nil && result.HasUpdate {
		fmt.Fprintln(os.Stderr)
		yellow.Fprintf(os.Stderr, "A new version of healthsync is available: %s → %s\n", Version, result.Latest)
		fmt.Fprintf(os.Stderr, "Update: curl -fsSL https://healthsync.sidv.dev/install | bash\n")
	}

	printSkillsStalenessNotice(homeDir)
}

// printSkillsStalenessNotice checks if installed skills are outdated.
func printSkillsStalenessNotice(homeDir string) {
	if Version == "" || Version == "dev" {
		return
	}
	targets := skills.InstalledTargets(skills.DefaultTargets(homeDir))
	for _, t := range targets {
		installed := skills.InstalledVersion(t)
		if installed != "" && installed != Version {
			yellow := color.New(color.FgYellow)
			fmt.Fprintln(os.Stderr)
			yellow.Fprintf(os.Stderr, "Installed skills are outdated (%s). Run: healthsync skills install\n", installed)
			return
		}
	}
}
