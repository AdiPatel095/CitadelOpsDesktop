package Logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	logFile *os.File
	mu      sync.Mutex
)

// InitLogger initializes the file logger. It keeps logging to stdout as well.
func InitLogger() error {
	mu.Lock()
	defer mu.Unlock()

	// Ensure Logs directory exists
	logsDir := "Logs"
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create Logs directory: %v", err)
	}

	// Create log file name with current date
	dateStr := time.Now().Format("2006-01-02")
	fileName := filepath.Join(logsDir, fmt.Sprintf("citadelops_%s.log", dateStr))

	// Open or create the log file
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}

	logFile = file

	// Create a MultiWriter to write to both stdout and the file
	// Actually `log` package does not cleanly support multiwriter globally without overwriting output
	// Alternatively, we wrap calls. But for global log.Print, we can set output.
	// We want to keep standard format.

	multi := &logWriter{}
	log.SetOutput(multi)

	return nil
}

// logWriter is a custom writer that writes to both os.Stdout and the logFile
type logWriter struct{}

func (w *logWriter) Write(p []byte) (n int, err error) {
	// Write to console
	n, err = os.Stdout.Write(p)

	// Write to file if it's open
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		_, _ = logFile.Write(p)
	}

	return n, err
}

// CloseLogger cleanly closes the log file
func CloseLogger() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}
