package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
)

var (
	logFile *os.File
	mu      sync.Mutex
	logger  *log.Logger
)

// Init initializes the logger to write to the specified file.
func Init(filename string) error {
	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	logFile = f
	logger = log.New(f, "", log.LstdFlags)
	return nil
}

// Close closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.Close()
	}
}

// Info logs an informational message.
func Info(format string, v ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		logger.Printf("[INFO] "+format, v...)
	}
}

// Error logs an error message.
func Error(format string, v ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		logger.Printf("[ERROR] "+format, v...)
	} else {
		// Fallback to stderr if logger not init
		fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", v...)
	}
}

// Debug logs a debug message.
func Debug(format string, v ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		logger.Printf("[DEBUG] "+format, v...)
	}
}
