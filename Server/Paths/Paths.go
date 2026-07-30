package Paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DataDirectoryEnvironment = "CITADEL_DATA_DIR"

func DataDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(DataDirectoryEnvironment)); configured != "" {
		absolute, err := filepath.Abs(configured)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", DataDirectoryEnvironment, err)
		}
		if err := os.MkdirAll(absolute, 0o755); err != nil {
			return "", fmt.Errorf("create data directory: %w", err)
		}
		return absolute, nil
	}

	root, err := instanceRoot()
	if err != nil {
		return "", err
	}
	dataDir := filepath.Join(root, "Data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("create data directory: %w", err)
	}
	return dataDir, nil
}

func instanceRoot() (string, error) {
	workingDirectory, workingErr := os.Getwd()
	if workingErr == nil {
		if _, err := os.Stat(filepath.Join(workingDirectory, "go.mod")); err == nil {
			return workingDirectory, nil
		}
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return filepath.Dir(absolute), nil
}
