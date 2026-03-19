package rorginerror

import (
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RorGinSpanError wraps a RorGinError and an OpenTelemetry span so that
// GinLogErrorAbort / GinLogErrorJSON automatically record the error on the
// span and set its status to codes.Error.
type RorGinSpanError struct {
	RorGinErrorData
	span trace.Span
}

// NewRorGinSpanErrorFromError creates a RorGinSpanError from an existing error.
func NewRorGinSpanErrorFromError(span trace.Span, status int, err error) RorGinError {
	return RorGinSpanError{
		RorGinErrorData: RorGinErrorData{
			RorError: rorerror.NewRorErrorFromError(status, err),
		},
		span: span,
	}
}

// NewRorGinSpanError creates a RorGinSpanError from a message string and optional underlying errors.
func NewRorGinSpanError(span trace.Span, status int, err string, errors ...error) RorGinError {
	return RorGinSpanError{
		RorGinErrorData: RorGinErrorData{
			RorError: rorerror.NewRorError(status, err, errors...),
		},
		span: span,
	}
}

func (e RorGinSpanError) GinLogErrorAbort(c *gin.Context, fields ...Field) {
	e.recordSpanError()
	e.RorGinErrorData.GinLogErrorAbort(c, fields...)
}

func (e RorGinSpanError) GinLogErrorJSON(c *gin.Context, fields ...Field) {
	e.recordSpanError()
	e.RorGinErrorData.GinLogErrorJSON(c, fields...)
}

func (e RorGinSpanError) recordSpanError() {
	if e.span == nil {
		return
	}
	for _, err := range e.GetErrors() {
		rortracer.SpanError(span, err)
	}
	e.span.SetStatus(codes.Error, e.GetMessage())
}

// GinHandleSpanErrorAndAbort is the span-aware equivalent of GinHandleErrorAndAbort.
// If err is non-nil it records the error on the span, logs it, and aborts the request.
func GinHandleSpanErrorAndAbort(c *gin.Context, span trace.Span, status int, err error, fields ...Field) bool {
	if err != nil {
		rorerror := NewRorGinSpanErrorFromError(span, status, err)
		fields = append(fields, rlog.Int("statuscode", status))
		rorerror.GinLogErrorAbort(c, fields...)
		return true
	}
	return false
}
