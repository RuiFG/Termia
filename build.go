//go:build ignore

// Termia cross-compilation build script.
//
// Usage:
//
//	go run build.go                       # build for current platform
//	go run build.go linux amd64           # cross-compile for linux/amd64
//	go run build.go linux arm64           # cross-compile for linux/arm64
//	go run build.go darwin amd64          # cross-compile for macOS Intel
//	go run build.go darwin arm64          # cross-compile for macOS Apple Silicon
//	go run build.go all                   # build all supported targets
//	go run build.go clean                 # remove all build artifacts
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Target represents a GOOS/GOARCH pair.
type Target struct {
	OS   string
	Arch string
}

// All supported targets.
var allTargets = []Target{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
}

var (
	// Module path — used for -ldflags version injection.
	versionPkg = "github.com/termia/termia/cmd"
	outputDir  = "dist"
)

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		// Default: build for current platform.
		build(Target{OS: runtime.GOOS, Arch: runtime.GOARCH})
		return
	}

	switch args[0] {
	case "all":
		buildAll()
	case "clean":
		clean()
	case "help", "-h", "--help":
		printUsage()
	default:
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: go run build.go <os> <arch>\n")
			fmt.Fprintf(os.Stderr, "       go run build.go all\n")
			fmt.Fprintf(os.Stderr, "       go run build.go clean\n")
			os.Exit(1)
		}
		t := Target{OS: args[0], Arch: args[1]}
		if !isValidTarget(t) {
			fmt.Fprintf(os.Stderr, "Unsupported target: %s/%s\n", t.OS, t.Arch)
			fmt.Fprintf(os.Stderr, "Supported targets:\n")
			for _, st := range allTargets {
				fmt.Fprintf(os.Stderr, "  %s %s\n", st.OS, st.Arch)
			}
			os.Exit(1)
		}
		build(t)
	}
}

func build(t Target) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fatal("create output dir: %v", err)
	}

	binaryName := binaryPath(t)
	version := getVersion()
	buildTime := time.Now().UTC().Format(time.RFC3339)

	ldflags := fmt.Sprintf("-s -w -X %s.Version=%s -X %s.BuildTime=%s",
		versionPkg, version, versionPkg, buildTime)

	// Pure Go build — no CGO required (using modernc.org/sqlite).
	fmt.Printf("Building termia for %s/%s → %s\n", t.OS, t.Arch, binaryName)

	cmd := exec.Command("go", "build",
		"-ldflags", ldflags,
		"-trimpath",
		"-o", binaryName,
		".",
	)
	cmd.Env = append(os.Environ(),
		"GOOS="+t.OS,
		"GOARCH="+t.Arch,
		"CGO_ENABLED=0",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	start := time.Now()
	if err := cmd.Run(); err != nil {
		fatal("build failed for %s/%s: %v", t.OS, t.Arch, err)
	}

	info, _ := os.Stat(binaryName)
	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Printf("  ✓ %s (%s, %s)\n", binaryName, formatSize(info.Size()), elapsed)
}

func buildAll() {
	fmt.Printf("Building termia for %d targets...\n\n", len(allTargets))
	for _, t := range allTargets {
		build(t)
		fmt.Println()
	}
	fmt.Println("All builds complete.")
}

func clean() {
	if err := os.RemoveAll(outputDir); err != nil {
		fatal("clean failed: %v", err)
	}
	// Also remove local binary if exists.
	for _, name := range []string{"termia", "termia.exe"} {
		os.Remove(name)
	}
	fmt.Println("Clean complete.")
}

func binaryPath(t Target) string {
	name := fmt.Sprintf("termia-%s-%s", t.OS, t.Arch)
	if t.OS == "windows" {
		name += ".exe"
	}
	return filepath.Join(outputDir, name)
}

func isValidTarget(t Target) bool {
	for _, st := range allTargets {
		if st.OS == t.OS && st.Arch == t.Arch {
			return true
		}
	}
	return false
}

func getVersion() string {
	// Try git describe first.
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return "dev"
}

func formatSize(bytes int64) string {
	const mb = 1024 * 1024
	if bytes >= mb {
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	}
	return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
}

func printUsage() {
	fmt.Println(`Termia Build Script

Usage:
  go run build.go                     Build for current platform
  go run build.go <os> <arch>         Cross-compile for specific target
  go run build.go all                 Build all supported targets
  go run build.go clean               Remove build artifacts

Supported targets:
  linux   amd64    Linux x86_64
  linux   arm64    Linux ARM64 (e.g. AWS Graviton)
  darwin  amd64    macOS Intel
  darwin  arm64    macOS Apple Silicon
  windows amd64    Windows x86_64

Examples:
  go run build.go linux amd64         Build Linux binary
  go run build.go darwin arm64        Build macOS ARM binary
  go run build.go all                 Build all platforms`)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
