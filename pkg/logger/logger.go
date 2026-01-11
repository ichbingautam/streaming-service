package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.SugaredLogger for structured logging
type Logger struct {
	*zap.SugaredLogger
}

// New creates a new Logger instance
func New(level, format string) *Logger {
	// Parse log level
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	// Create encoder config
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// Create encoder based on format
	var encoder zapcore.Encoder
	if format == "console" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Create core
	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		zapLevel,
	)

	// Create logger with caller info
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	return &Logger{logger.Sugar()}
}

// WithFields returns a new Logger with additional fields
func (l *Logger) WithFields(fields ...interface{}) *Logger {
	return &Logger{l.SugaredLogger.With(fields...)}
}

// WithError returns a new Logger with error field
func (l *Logger) WithError(err error) *Logger {
	return &Logger{l.SugaredLogger.With("error", err.Error())}
}
