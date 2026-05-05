package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// NewLogger 创建新的日志器
func NewLogger(debug bool) (*zap.SugaredLogger, error) {
	// 配置编码器
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 创建编码器
	var encoder zapcore.Encoder
	if debug {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// 配置日志级别
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}

	// 配置输出
	var writeSyncer zapcore.WriteSyncer
	if debug {
		// 开发环境输出到控制台
		writeSyncer = zapcore.AddSync(os.Stdout)
	} else {
		// 生产环境输出到文件
		lumberJackLogger := &lumberjack.Logger{
			Filename:   "logs/app.log",
			MaxSize:    10, // megabytes
			MaxBackups: 5,
			MaxAge:     30, // days
			Compress:   true,
		}
		writeSyncer = zapcore.AddSync(lumberJackLogger)
	}

	// 创建核心日志器
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// 创建日志器
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	return logger.Sugar(), nil
}

var logger *zap.Logger
var sugarLogger *zap.SugaredLogger

func GetLogger() *zap.SugaredLogger {
	if sugarLogger != nil {
		return sugarLogger
	}

	logLevel := zap.NewAtomicLevelAt(getLogLevel(os.Getenv("LOG_LEVEL")))
	encoder := getEncoder()
	writerSyncer := getLogWriter()
	fileCore := zapcore.NewCore(encoder, writerSyncer, logLevel)
	consoleOutput := zapcore.Lock(os.Stdout)
	consoleCore := zapcore.NewCore(
		encoder,
		consoleOutput,
		logLevel,
	)
	core := zapcore.NewTee(fileCore, consoleCore)
	logger = zap.New(core, zap.AddCaller())
	sugarLogger = logger.Sugar()
	return sugarLogger
}

// core 三个参数之  编码
func getEncoder() zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
	}
	return zapcore.NewConsoleEncoder(encoderConfig)
}

func getLogWriter() zapcore.WriteSyncer {
	lumberJackLogger := &lumberjack.Logger{
		Filename:   "logs/app.log",
		MaxSize:    10,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   false,
	}
	return zapcore.AddSync(lumberJackLogger)
}

func getLogLevel(level string) zapcore.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return zapcore.DebugLevel
	case "WARN":
		return zapcore.WarnLevel
	case "ERROR":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
