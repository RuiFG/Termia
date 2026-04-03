package cmd

import (
	"testing"

	"github.com/adrg/xdg"
)

func TestPrepareStartsModelsCatalogRefresh(t *testing.T) {
	oldStart := startModelsCatalogRefresh
	oldVerbose := verbose
	oldLogger := logger
	oldCfg := cfg
	oldDataHome := xdg.DataHome
	t.Cleanup(func() {
		startModelsCatalogRefresh = oldStart
		verbose = oldVerbose
		logger = oldLogger
		cfg = oldCfg
		xdg.DataHome = oldDataHome
	})

	xdg.DataHome = t.TempDir()
	verbose = false
	called := 0
	startModelsCatalogRefresh = func() {
		called++
	}

	if err := prepare(); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected one refresh startup call, got %d", called)
	}
}
