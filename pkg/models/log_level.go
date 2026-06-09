package models

const (
	//tygo:emit export type LogLevel = typeof LogLevelDebug | typeof LogLevelInfo | typeof LogLevelWarn | typeof LogLevelError | typeof LogLevelFatal;
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
	LogLevelFatal = "fatal"
)
