package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/termia/termia/internal/db"
)

func waitForPendingPromptResponse(ctx context.Context, database *db.DB, promptID string) (string, error) {
	if database == nil {
		return "", fmt.Errorf("database is nil")
	}
	promptID = strings.TrimSpace(promptID)
	if promptID == "" {
		return "", fmt.Errorf("prompt id is empty")
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		prompt, err := database.GetPendingPrompt(promptID)
		if err != nil {
			return "", err
		}
		if strings.EqualFold(prompt.Status, db.PendingPromptStatusResolved) {
			return strings.TrimSpace(prompt.ResponseJSON), nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}
