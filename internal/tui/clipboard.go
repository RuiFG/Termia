package tui

import (
	"encoding/base64"
	"os"
)

func copyToClipboard(text string) error {
	if text == "" {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	seq := "\x1b]52;c;" + encoded + "\x07"
	_, err := os.Stdout.Write([]byte(seq))
	return err
}
