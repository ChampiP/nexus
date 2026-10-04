package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpDoesNotOpenDatabase(t *testing.T) {
	for _, args := range [][]string{
		{"--help"},
		{"help"},
		{"-h"},
		{"org", "--help"},
		{"pausa", "--help"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			home := t.TempDir()
			dbDir := filepath.Join(t.TempDir(), "data")
			dbPath := filepath.Join(dbDir, "nexus.db")
			t.Setenv("HOME", home)
			t.Setenv("NEXUS_DB", dbPath)

			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			originalStdout := os.Stdout
			os.Stdout = writer
			runErr := run(args)
			os.Stdout = originalStdout
			if closeErr := writer.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			output, readErr := io.ReadAll(reader)
			_ = reader.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if runErr != nil {
				t.Fatalf("run() error = %v", runErr)
			}
			if !strings.Contains(string(output), "Uso: nexus") {
				t.Fatalf("output lacks usage text: %s", output)
			}
			if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
				t.Errorf("database path exists or could not be checked: %v", err)
			}
			if _, err := os.Stat(dbDir); !os.IsNotExist(err) {
				t.Errorf("database directory exists or could not be checked: %v", err)
			}
		})
	}
}
