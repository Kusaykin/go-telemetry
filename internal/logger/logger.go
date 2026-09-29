package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewJSON(level string) (*zap.Logger, error) {
	return build(zap.NewProductionConfig(), level)
}

func NewConsole(level string) (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()
	cfg.Development = false
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.DisableCaller = true

	return build(cfg, level)
}

func build(cfg zap.Config, level string) (*zap.Logger, error) {
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return nil, err
	}

	cfg.Level = lvl
	cfg.DisableStacktrace = true

	return cfg.Build()
}
