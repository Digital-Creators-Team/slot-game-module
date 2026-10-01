package logging

import (
	"context"

	"github.com/rs/zerolog"
)

type LoggerProvider interface {
	For(ctx context.Context) *zerolog.Logger
	With(withFunc func(loggerContext zerolog.Context) zerolog.Context) LoggerProvider
}

type loggerProvider struct {
	logger zerolog.Logger
}

func NewLoggerProvider(
	logger zerolog.Logger,
) LoggerProvider {
	return &loggerProvider{
		logger: logger,
	}
}

func (p *loggerProvider) For(ctx context.Context) *zerolog.Logger {
	logger := p.logger.With().Ctx(ctx).Logger()
	return &logger
}

func (p *loggerProvider) With(
	withFunc func(loggerContext zerolog.Context) zerolog.Context,
) LoggerProvider {
	return NewLoggerProvider(withFunc(p.logger.With()).Logger())
}
