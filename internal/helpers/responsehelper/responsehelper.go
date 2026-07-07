// responsehelper is a package that simplifies http response handling in a consistent format.
package responsehelper

import (
	"github.com/NorskHelsenett/ror-api/internal/models/responses"
	"github.com/gin-gonic/gin"
)

// ErrorResponse is a helper function to send error responses in a consistent format where the Data field only contains the error message.
type (
	errorResponse struct {
		Message string
		Data    map[string]any
	}
	ErrorResponseOption func(*errorResponse)
)

func newErrorResponse() *errorResponse {
	return &errorResponse{
		Message: "error",
		Data:    nil,
	}
}

func WithMessage(message string) ErrorResponseOption {
	return func(er *errorResponse) {
		er.Message = message
	}
}

func WithData(data map[string]any) ErrorResponseOption {
	return func(er *errorResponse) {
		er.Data = data
	}
}

func ErrorResponse(c *gin.Context, statusCode int, err error, options ...ErrorResponseOption) {
	errMessage := newErrorResponse()
	for _, option := range options {
		option(errMessage)
	}

	if err != nil {
		errMessage.Data["data"] = err.Error()
	}

	c.JSON(statusCode, responses.Cluster{Status: statusCode, Message: errMessage.Message, Data: errMessage.Data})
}
