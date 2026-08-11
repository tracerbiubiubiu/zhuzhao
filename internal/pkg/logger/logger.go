package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// New 创建 slog Logger，同时输出到文件和控制台
func New(cfg config.LogConfig) *slog.Logger {
	var slogLevel slog.Level
	switch cfg.Level {
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
		Filename:   filepath.Join(cfg.Dir, "app.log"),
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   true,
	}

	multiWriter := io.MultiWriter(writer, os.Stdout)

	handler := slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	})

	return slog.New(handler)
}
