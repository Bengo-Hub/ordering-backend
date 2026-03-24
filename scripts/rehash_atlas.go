//go:build ignore

// rehash_atlas.go recomputes atlas.sum for the migrations directory.
// Run from ordering-backend root: go run ./scripts/rehash_atlas.go
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	dir := "internal/ent/migrate/migrations"

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("Error reading dir:", err)
		os.Exit(1)
	}

	var lines []string
	var fileHashes []string

	// Sort entries by name (deterministic order)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if e.IsDir() || e.Name() == "atlas.sum" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", e.Name(), err)
			os.Exit(1)
		}
		h := sha256.Sum256(data)
		hash := "h1:" + base64.StdEncoding.EncodeToString(h[:])
		lines = append(lines, e.Name()+" "+hash)
		fileHashes = append(fileHashes, hash)
	}

	// Compute overall hash (hash of all file hashes)
	allHashes := strings.Join(fileHashes, "\n") + "\n"
	overall := sha256.Sum256([]byte(allHashes))
	overallHash := "h1:" + base64.StdEncoding.EncodeToString(overall[:])

	// Write atlas.sum
	content := overallHash + "\n" + strings.Join(lines, "\n") + "\n"
	err = os.WriteFile(filepath.Join(dir, "atlas.sum"), []byte(content), 0644)
	if err != nil {
		fmt.Println("Error writing atlas.sum:", err)
		os.Exit(1)
	}
	fmt.Printf("atlas.sum written with %d migration files\n", len(lines))
}
