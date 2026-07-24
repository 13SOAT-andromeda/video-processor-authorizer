package utils

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

// ddHandler wraps an slog.Handler (JSON) and, on every record, injects
// dd.trace_id / dd.span_id attributes extracted from the active APM span
// carried in ctx, when present. dd-trace-go v2 does not do this
// automatically for log/slog (unlike some other Datadog tracers), so it is
// done here explicitly. This is what lets DD_LOGS_INJECTION=true correlate
// a CloudWatch log line with the Lambda invocation's trace in the Datadog
// UI. IDs are encoded as decimal strings (not JSON numbers) because a
// uint64 trace ID can lose precision if a downstream consumer parses the
// JSON number as a float64.
type ddHandler struct {
	slog.Handler
}

func (h ddHandler) Handle(ctx context.Context, r slog.Record) error {
	if span, ok := tracer.SpanFromContext(ctx); ok {
		sc := span.Context()
		r.AddAttrs(
			slog.String("dd.trace_id", strconv.FormatUint(sc.TraceIDLower(), 10)),
			slog.String("dd.span_id", strconv.FormatUint(sc.SpanID(), 10)),
		)
	}
	return h.Handler.Handle(ctx, r)
}

// Logger preserves the Printf-style call surface previously provided by the
// stdlib *log.Logger based InfoLogger/ErrorLogger, so existing call sites
// (utils.InfoLogger.Printf(...) / utils.ErrorLogger.Printf(...)) keep
// compiling unchanged. Output is now structured JSON instead of plain text,
// which is required for the Datadog Lambda Extension / DD_LOGS_INJECTION to
// parse and correlate log lines with traces.
type Logger struct {
	base  *slog.Logger
	level slog.Level
}

// Printf logs a formatted message with no request/trace context available.
// Prefer PrintfContext in request-scoped code paths so the log line can be
// correlated with the active trace.
func (l *Logger) Printf(format string, args ...any) {
	l.base.Log(context.Background(), l.level, fmt.Sprintf(format, args...))
}

// PrintfContext behaves like Printf but additionally injects dd.trace_id /
// dd.span_id attributes extracted from ctx's active span, when present.
func (l *Logger) PrintfContext(ctx context.Context, format string, args ...any) {
	l.base.Log(ctx, l.level, fmt.Sprintf(format, args...))
}

var (
	InfoLogger  *Logger
	ErrorLogger *Logger
)

func init() {
	// dd.service / dd.env / dd.version are attached once here (rather than
	// per log line) so every record is taggable/filterable in Datadog Log
	// Management even before trace correlation kicks in.
	staticAttrs := []any{
		"dd.service", os.Getenv("DD_SERVICE"),
		"dd.env", os.Getenv("DD_ENV"),
		"dd.version", os.Getenv("DD_VERSION"),
	}

	InfoLogger = &Logger{
		base:  slog.New(ddHandler{slog.NewJSONHandler(os.Stdout, nil)}).With(staticAttrs...),
		level: slog.LevelInfo,
	}
	ErrorLogger = &Logger{
		base:  slog.New(ddHandler{slog.NewJSONHandler(os.Stderr, nil)}).With(staticAttrs...),
		level: slog.LevelError,
	}
}
