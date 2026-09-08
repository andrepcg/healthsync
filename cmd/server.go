package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/BRO3886/healthsync/internal/people"
	"github.com/BRO3886/healthsync/internal/server"
)

var (
	serverPort int
	serverHost string
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the web dashboard and HTTP API",
	Long: `Start the HTTP server: a multi-person health dashboard (web UI) plus a JSON
API under /api. Each person gets their own SQLite database under
<data-dir>/people/<id>.db; uploads are imported in the background.

The data directory defaults to $HEALTHSYNC_DATA_DIR, then ~/.healthsync.`,
	RunE: runServer,
}

func init() {
	serverCmd.Flags().IntVar(&serverPort, "port", 8080, "port to listen on")
	serverCmd.Flags().StringVar(&serverHost, "host", "0.0.0.0", "host to bind to")
	rootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) error {
	store, err := people.Open(dataDir)
	if err != nil {
		return fmt.Errorf("opening data dir: %w", err)
	}
	defer store.Close()

	fmt.Printf("Data dir: %s\n", dataDir)

	return server.Start(server.Config{
		Host:    serverHost,
		Port:    serverPort,
		Store:   store,
		Version: Version,
	})
}
