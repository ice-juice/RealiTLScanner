package scan

import "log/slog"

func loggerOrDiscard(log *slog.Logger) *slog.Logger {
	if log != nil {
		return log
	}
	return slog.New(slog.DiscardHandler)
}
