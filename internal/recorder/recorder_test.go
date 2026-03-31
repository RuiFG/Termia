package recorder

import (
	"testing"

	"go.uber.org/zap"
)

func TestShouldRecordCommandIgnoresTUI(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{command: "tui", want: false},
		{command: "tui --team ops", want: false},
		{command: "termia tui", want: false},
		{command: ".\\termia.exe tui --team ops", want: false},
		{command: "tai summarize", want: true},
		{command: "ls -la", want: true},
	}

	for _, tt := range tests {
		if got := shouldRecordCommand(tt.command); got != tt.want {
			t.Fatalf("shouldRecordCommand(%q) = %v, want %v", tt.command, got, tt.want)
		}
	}
}

func TestRecorderIgnoresTUIStartAndEndMarkers(t *testing.T) {
	r, err := New(nil, t.TempDir(), zap.NewNop())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer r.Close()

	if err := r.processMarker(&Marker{CmdID: "cmd-1", Phase: PhaseStart, Command: "tui"}); err != nil {
		t.Fatalf("start marker error = %v", err)
	}

	active, ok := r.commands["cmd-1"]
	if !ok {
		t.Fatalf("expected ignored command to remain tracked until end marker")
	}
	if !active.ignored {
		t.Fatalf("expected command to be marked ignored")
	}

	if err := r.processMarker(&Marker{CmdID: "cmd-1", Phase: PhaseEnd}); err != nil {
		t.Fatalf("end marker error = %v", err)
	}
	if _, exists := r.commands["cmd-1"]; exists {
		t.Fatalf("expected ignored command to be removed after end marker")
	}
}
