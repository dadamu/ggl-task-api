// Package apierror defines the API error format and maps errors to HTTP
// responses in one place (ADR-0004).
package apierror

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	"github.com/dadamu/ggl-task-api/internal/domain/task"
	"github.com/dadamu/ggl-task-api/internal/transport/http/dto"
)

// Machine-readable error codes.
const (
	CodeInvalidJSON      = "INVALID_JSON"
	CodeValidationFailed = "VALIDATION_FAILED"
	CodeInvalidID        = "INVALID_ID"
	CodeTaskNotFound     = "TASK_NOT_FOUND"
	CodeRouteNotFound    = "ROUTE_NOT_FOUND"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeVersionConflict  = "VERSION_CONFLICT"
	CodeInternal         = "INTERNAL_ERROR"

	CodeUnsupportedMediaType = "UNSUPPORTED_MEDIA_TYPE"
	CodePayloadTooLarge      = "PAYLOAD_TOO_LARGE"
)

// Response is the error response body.
type Response struct {
	Error Body `json:"error"`
}

// Body describes an error.
type Body struct {
	Code    string        `json:"code" example:"VALIDATION_FAILED"`
	Message string        `json:"message" example:"invalid request body"`
	Details []FieldDetail `json:"details,omitempty"`
	Current *dto.Task     `json:"current,omitempty"` // set on VERSION_CONFLICT
}

// FieldDetail describes one invalid field.
type FieldDetail struct {
	Field  string `json:"field" example:"name"`
	Reason string `json:"reason" example:"must not be blank"`
}

// Abort writes an error response and stops the handler chain.
func Abort(c *gin.Context, status int, body Body) {
	c.AbortWithStatusJSON(status, Response{Error: body})
}

// FromError maps an application or domain error to a response.
// Unmapped errors are logged and returned as 500 without internal details.
func FromError(c *gin.Context, err error) {
	var verr *task.ValidationError
	var cerr *task.VersionConflictError
	switch {
	case errors.As(err, &verr):
		details := make([]FieldDetail, 0, len(verr.Fields))
		for _, f := range verr.Fields {
			details = append(details, FieldDetail{Field: f.Field, Reason: f.Reason})
		}
		Abort(c, http.StatusBadRequest, Body{Code: CodeValidationFailed, Message: "invalid request body", Details: details})
	case errors.Is(err, task.ErrNotFound):
		Abort(c, http.StatusNotFound, Body{Code: CodeTaskNotFound, Message: "task not found"})
	case errors.As(err, &cerr):
		current := dto.FromTask(cerr.Current)
		Abort(c, http.StatusConflict, Body{
			Code:    CodeVersionConflict,
			Message: "task has been modified; reload and retry with the current version",
			Current: &current,
		})
	default:
		Internal(c, err)
	}
}

// Internal logs err and writes a generic 500 response.
func Internal(c *gin.Context, err error) {
	slog.ErrorContext(c.Request.Context(), "unhandled error",
		"err", err, "method", c.Request.Method, "path", c.Request.URL.Path)
	AbortInternal(c)
}

// AbortInternal writes a generic 500 response without logging. Use it when the
// error has already been logged (e.g. by gin's recovery, which includes the stack trace).
func AbortInternal(c *gin.Context) {
	Abort(c, http.StatusInternalServerError, Body{Code: CodeInternal, Message: "internal server error"})
}

// FromBindError maps a request decoding or binding error to a 400 response.
func FromBindError(c *gin.Context, err error) {
	var verrs validator.ValidationErrors
	var typeErr *json.UnmarshalTypeError
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr):
		Abort(c, http.StatusRequestEntityTooLarge, Body{Code: CodePayloadTooLarge, Message: "request body is too large"})
	case errors.As(err, &verrs):
		details := make([]FieldDetail, 0, len(verrs))
		for _, fe := range verrs {
			details = append(details, FieldDetail{Field: fe.Field(), Reason: "is required"})
		}
		Abort(c, http.StatusBadRequest, Body{Code: CodeValidationFailed, Message: "invalid request body", Details: details})
	case errors.As(err, &typeErr) && typeErr.Field == "":
		// Top-level type mismatch, e.g. an array instead of an object.
		Abort(c, http.StatusBadRequest, Body{Code: CodeInvalidJSON, Message: "request body must be a JSON object"})
	case errors.As(err, &typeErr):
		Abort(c, http.StatusBadRequest, Body{
			Code:    CodeValidationFailed,
			Message: "invalid request body",
			Details: []FieldDetail{{Field: typeErr.Field, Reason: "must be " + jsonType(typeErr.Type)}},
		})
	case errors.Is(err, io.EOF):
		Abort(c, http.StatusBadRequest, Body{Code: CodeInvalidJSON, Message: "request body must not be empty"})
	default:
		Abort(c, http.StatusBadRequest, Body{Code: CodeInvalidJSON, Message: "request body is not valid JSON"})
	}
}

func jsonType(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "an integer"
	case reflect.String:
		return "a string"
	default:
		return fmt.Sprintf("of type %s", t.Kind())
	}
}

var registerJSONNames sync.Once

// UseJSONFieldNames makes gin's validator report JSON field names ("name")
// instead of Go struct field names ("Name") in binding errors. It configures a
// process-wide validator and is safe to call multiple times.
func UseJSONFieldNames() {
	registerJSONNames.Do(func() {
		v, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				return ""
			}
			return name
		})
	})
}
