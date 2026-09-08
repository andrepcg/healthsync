package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/BRO3886/healthsync/internal/parser"
	"github.com/BRO3886/healthsync/internal/storage"
)

var verbose bool

var parseCmd = &cobra.Command{
	Use:   "parse <file.zip|export.xml>",
	Short: "Parse an Apple Health export file into the database",
	Args:  cobra.ExactArgs(1),
	RunE:  runParse,
}

func init() {
	parseCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging")
	rootCmd.AddCommand(parseCmd)
}

func runParse(cmd *cobra.Command, args []string) error {
	path := args[0]

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", path)
	}
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	dbPath, err := resolveDBPath()
	if err != nil {
		return err
	}
	if verbose {
		log.Printf("[verbose] input file: %s (%.2f MB)", path, float64(info.Size())/(1024*1024))
		log.Printf("[verbose] database: %s", dbPath)
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	fmt.Printf("Parsing %s...\n", path)
	fmt.Printf("Database: %s\n\n", dbPath)

	start := time.Now()
	lastLog := time.Now()
	spinFrames := []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	frame := 0

	progress := func(p parser.Progress) {
		spin := spinFrames[frame%len(spinFrames)]
		frame++
		elapsed := time.Since(start).Round(time.Second)
		rate := float64(p.Records) / time.Since(start).Seconds()
		fmt.Fprintf(os.Stderr, "\r  %s %s records · %s workouts · %s routes · %s ECGs · %.0f/s · %s   ",
			spin,
			formatCommas(p.Records),
			formatCommas(p.Workouts),
			formatCommas(p.Routes),
			formatCommas(p.ECGs),
			rate,
			elapsed,
		)
		if verbose && time.Since(lastLog) > 5*time.Second {
			elapsed := time.Since(start).Round(time.Millisecond)
			log.Printf("[verbose] progress: %d records, %d workouts (%.0f/s, elapsed %s)", p.Records, p.Workouts, rate, elapsed)
			lastLog = time.Now()
		}
	}

	if verbose {
		log.Printf("[verbose] starting XML parse...")
	}

	importID, importErr := db.BeginImport(filepath.Base(path), info.Size(), start.UTC().Format(time.RFC3339))

	result, err := parser.ParseFile(path, db, progress)

	if importErr == nil {
		row := storage.ImportRow{FinishedAt: time.Now().UTC().Format(time.RFC3339), Status: "completed"}
		if err != nil {
			row.Status, row.Error = "failed", err.Error()
		}
		if result != nil {
			row.ExportDate, row.Locale = result.ExportDate, result.Locale
			row.Records, row.Workouts, row.Routes = result.Records, result.Workouts, result.Routes
			row.ECGs, row.ActivityDays, row.HRVBeats, row.Errors = result.ECGs, result.ActivityDays, result.HRVBeats, result.Errors
			if b, jerr := json.Marshal(result.Stats); jerr == nil {
				row.TableStats = string(b)
			}
		}
		db.FinishImport(importID, row)
	}
	if err != nil {
		return fmt.Errorf("parsing file: %w", err)
	}

	elapsed := time.Since(start)

	fmt.Fprintf(os.Stderr, "\r\033[2K") // clear the spinner line
	fmt.Printf("  Records:        %s\n", formatCommas(result.Records))
	fmt.Printf("  Workouts:       %s\n", formatCommas(result.Workouts))
	fmt.Printf("  Routes:         %s (%s points)\n", formatCommas(result.Routes), formatCommas(result.RoutePoints))
	fmt.Printf("  ECGs:           %s\n", formatCommas(result.ECGs))
	fmt.Printf("  Activity days:  %s\n", formatCommas(result.ActivityDays))
	fmt.Printf("  HRV beats:      %s\n", formatCommas(result.HRVBeats))
	if result.Errors > 0 {
		fmt.Printf("  Warnings:       %s\n", formatCommas(result.Errors))
		for _, w := range result.Warnings {
			fmt.Printf("    - %s\n", w)
		}
	}
	if result.ExportDate != "" {
		fmt.Printf("  Export date:    %s\n", result.ExportDate)
	}
	fmt.Printf("  Duration:       %s\n\n", elapsed.Round(time.Millisecond))

	if verbose {
		rate := float64(result.Total+result.Workouts) / elapsed.Seconds()
		log.Printf("[verbose] parse complete: %.0f records/sec", rate)
	}

	// Print per-table insert stats for this run
	fmt.Println("Rows written this run (inserted / skipped as duplicates):")
	for _, st := range result.Stats {
		fmt.Printf("  %-36s %10s / %s\n", st.Table, formatCommas(st.Inserted), formatCommas(st.Skipped))
	}

	return nil
}

// formatCommas formats an int64 with thousands separators (e.g. 1234567 → "1,234,567").
func formatCommas(n int64) string {
	s := strconv.FormatInt(n, 10)
	out := make([]byte, 0, len(s)+(len(s)-1)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}
