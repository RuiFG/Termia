package cmd

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/config"
	_ "modernc.org/sqlite"
)

var cleanCmd *cobra.Command
var cleanDryRun bool
var cleanOlderThan int
var cleanAll bool

func init() {
	cleanCmd = &cobra.Command{
		Use:   "clean",
		Short: "Clean old transcripts and data",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClean()
		},
	}

	cleanCmd.Flags().BoolVar(
		&cleanDryRun,
		"dry-run",
		false,
		"show what would be deleted without deleting",
	)

	cleanCmd.Flags().IntVar(
		&cleanOlderThan,
		"older-than",
		0,
		"delete files older than N days (default: config MaxTranscriptAgeDays)",
	)

	cleanCmd.Flags().BoolVar(
		&cleanAll,
		"all",
		false,
		"delete all data (requires confirmation)",
	)

	rootCmd.AddCommand(cleanCmd)
}

func runClean() error {
	// If --all: prompt for confirmation
	if cleanAll {
		fmt.Print("Are you sure? This will delete ALL data. Type 'yes' to confirm: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		if strings.TrimSpace(input) != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Determine age threshold
	var olderThanDays int
	if cleanOlderThan > 0 {
		olderThanDays = cleanOlderThan
	} else {
		olderThanDays = cfg.General.MaxTranscriptAgeDays
	}

	// Calculate cutoff time
	cutoffTime := time.Now().AddDate(0, 0, -olderThanDays)
	cutoffUnix := cutoffTime.Unix()

	transcriptsDir := config.TranscriptsDir()

	if cleanAll {
		// Delete everything in transcripts directory
		return deleteAllTranscripts(transcriptsDir)
	}

	// Walk transcripts directory and find old files
	var filesToDelete []string
	var totalSize int64

	err := filepath.Walk(transcriptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.ModTime().Unix() < cutoffUnix {
			filesToDelete = append(filesToDelete, path)
			totalSize += info.Size()
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk transcripts directory: %w", err)
	}

	// If dry-run, print files and exit
	if cleanDryRun {
		fmt.Printf("Found %d files to delete (%s)\n", len(filesToDelete), formatBytes(totalSize))
		for _, f := range filesToDelete {
			fmt.Printf("  %s\n", f)
		}
		return nil
	}

	// Delete files
	deletedCount := 0
	deletedSize := int64(0)

	for _, f := range filesToDelete {
		if err := os.Remove(f); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete %s: %v\n", f, err)
			continue
		}
		if info, err := os.Stat(f); err == nil {
			deletedSize += info.Size()
		}
		deletedCount++
	}

	fmt.Printf("Deleted %d files (%s)\n", deletedCount, formatBytes(deletedSize))

	// Optionally VACUUM the database if --all
	if cleanAll {
		if err := vacuumDatabase(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to vacuum database: %v\n", err)
		} else {
			fmt.Println("Database vacuumed")
		}
	}

	return nil
}

func deleteAllTranscripts(transcriptsDir string) error {
	var totalSize int64
	var deletedCount int

	err := filepath.Walk(transcriptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			totalSize += info.Size()
			if err := os.Remove(path); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to delete %s: %v\n", path, err)
				return nil
			}
			deletedCount++
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk transcripts directory: %w", err)
	}

	fmt.Printf("Deleted %d files (%s)\n", deletedCount, formatBytes(totalSize))

	// VACUUM database
	if err := vacuumDatabase(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to vacuum database: %v\n", err)
	} else {
		fmt.Println("Database vacuumed")
	}

	return nil
}

func vacuumDatabase() error {
	dbPath := config.DBPath()
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer conn.Close()

	if _, err := conn.Exec("VACUUM"); err != nil {
		return fmt.Errorf("failed to vacuum database: %w", err)
	}

	return nil
}
