package loggers

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileLogger struct {
	filePath string
	maxSize  int64 // 最大文件大小(MB)
	backups  int   // 保留的备份数量
	file     *os.File
	mu       sync.Mutex
}

func NewFileLogger(filePath string, maxSize int64, backups int) (*FileLogger, error) {
	logger := &FileLogger{
		filePath: filePath,
		maxSize:  maxSize * 1024 * 1024, // 转换为字节
		backups:  backups,
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if err := logger.openFile(); err != nil {
		return nil, err
	}

	return logger, nil
}

// openFile 打开日志文件。调用方必须已持有 l.mu 锁(L-05: 避免 rotate→openFile 重入死锁)。
func (l *FileLogger) openFile() error {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(l.filePath), 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %v", err)
	}

	file, err := os.OpenFile(l.filePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}

	l.file = file
	return nil
}

func (l *FileLogger) rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 关闭当前文件
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			return err
		}
	}

	// 重命名现有日志文件
	for i := l.backups; i > 0; i-- {
		oldPath := l.backupPath(i - 1)
		newPath := l.backupPath(i)

		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Rename(oldPath, newPath); err != nil {
				return err
			}
		}
	}

	// 重命名当前日志文件
	if err := os.Rename(l.filePath, l.backupPath(0)); err != nil && !os.IsNotExist(err) {
		return err
	}

	// 创建新日志文件
	return l.openFile()
}

func (l *FileLogger) backupPath(num int) string {
	if num == 0 {
		return l.filePath + ".1"
	}
	return fmt.Sprintf("%s.%d", l.filePath, num+1)
}

func (l *FileLogger) checkSize() bool {
	info, err := os.Stat(l.filePath)
	if err != nil {
		return false
	}
	return info.Size() >= l.maxSize
}

func (l *FileLogger) Write(p []byte) (n int, err error) {
	if l.checkSize() {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.file.Write(p)
}

func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *FileLogger) WriteLog(level, message string) {
	logEntry := fmt.Sprintf("[%s] [%s] %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		level,
		message)

	l.Write([]byte(logEntry))
}
