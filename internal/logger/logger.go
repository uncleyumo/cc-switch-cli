package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cc-switch-cli/internal/config"
)

func LogSwitch(provider string) error {
	logPath := filepath.Join(config.ConfigDir(), "model-switch.logs")

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	line := fmt.Sprintf("%s | %s\n", time.Now().Format("2006-01-02 15:04:05"), provider)
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("failed to write log: %w", err)
	}

	return nil
}
