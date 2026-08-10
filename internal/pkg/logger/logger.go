package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// New 创建 slog Logger，同时输出到文件和控制台
func New(level, logDir string, maxSize, maxBackups, maxAge int) *slog.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	writer := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "app.log"),
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   true,
	}

	multiWriter := io.MultiWriter(writer, os.Stdout)

	handler := slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	})

	return slog.New(handler)
}
