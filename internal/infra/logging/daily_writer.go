// Package logging 提供应用日志输出能力。
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const dateLayout = "2006-01-02"

// DailyWriter 将当前日志写入稳定路径，并在跨日后按日期归档。
type DailyWriter struct {
	mu      sync.Mutex
	path    string
	now     func() time.Time
	file    *os.File
	fileDay string
}

// NewDailyWriter 创建按自然日轮转的 writer。
func NewDailyWriter(path string) (*DailyWriter, error) {
	return newDailyWriter(path, time.Now)
}

func newDailyWriter(path string, now func() time.Time) (*DailyWriter, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("log file path is required")
	}
	if now == nil {
		return nil, fmt.Errorf("clock is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	writer := &DailyWriter{path: path, now: now}
	if err := writer.openCurrent(); err != nil {
		return nil, err
	}
	return writer, nil
}

// Write 实现 io.Writer。
func (writer *DailyWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	today := writer.now().Format(dateLayout)
	if writer.file == nil || writer.fileDay != today {
		if err := writer.rotate(today); err != nil {
			return 0, err
		}
	}
	return writer.file.Write(data)
}

// Close 关闭当前日志文件。
func (writer *DailyWriter) Close() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	if writer.file == nil {
		return nil
	}
	err := writer.file.Close()
	writer.file = nil
	return err
}

func (writer *DailyWriter) openCurrent() error {
	now := writer.now()
	today := now.Format(dateLayout)
	if info, err := os.Stat(writer.path); err == nil {
		fileDay := info.ModTime().In(now.Location()).Format(dateLayout)
		if fileDay != today {
			if err := writer.archive(fileDay); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat current log file: %w", err)
	}

	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open current log file: %w", err)
	}
	writer.file = file
	writer.fileDay = today
	return nil
}

func (writer *DailyWriter) rotate(today string) error {
	if writer.file != nil {
		if err := writer.file.Close(); err != nil {
			writer.file = nil
			return fmt.Errorf("close current log file: %w", err)
		}
		writer.file = nil
	}
	if writer.fileDay != "" && writer.fileDay != today {
		if err := writer.archive(writer.fileDay); err != nil {
			return err
		}
		writer.fileDay = ""
	}

	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open rotated log file: %w", err)
	}
	writer.file = file
	writer.fileDay = today
	return nil
}

func (writer *DailyWriter) archive(day string) error {
	archivePath, err := writer.nextArchivePath(day)
	if err != nil {
		return err
	}
	if err := os.Rename(writer.path, archivePath); err != nil {
		return fmt.Errorf("archive log file: %w", err)
	}
	return nil
}

func (writer *DailyWriter) nextArchivePath(day string) (string, error) {
	extension := filepath.Ext(writer.path)
	base := strings.TrimSuffix(writer.path, extension) + "-" + day
	for index := 0; ; index++ {
		candidate := base + extension
		if index > 0 {
			candidate = base + "." + strconv.Itoa(index) + extension
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("stat archived log file: %w", err)
		}
	}
}
