package resp

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

type Response struct {
	Status           string            `json:"status"`
	Error            string            `json:"error,omitempty"`
	ValidationErrors map[string]string `json:"validation_errors,omitempty"`
}

const (
	StatusOK    = "OK"
	StatusError = "Error"
)

func OK() Response {
	return Response{
		Status: StatusOK,
	}
}

func Error(msg string) Response {
	return Response{
		Status: StatusError,
		Error:  msg,
	}
}

func ValidationError(errs validator.ValidationErrors) Response {
	errMsgs := make(map[string]string)

	for _, err := range errs {
		field := strings.ToLower(err.Field())
		switch err.ActualTag() {
		case "required":
			errMsgs[field] = fmt.Sprintf("field %s is a required field", field)
		case "url":
			errMsgs[field] = fmt.Sprintf("field %s is not a valid URL", field)
		case "email":
			errMsgs[field] = fmt.Sprintf("field %s is not a valid email", field)
		case "oneof":
			errMsgs[field] = fmt.Sprintf("field %s must be one of: %s", field, err.Param())
		case "min":
			errMsgs[field] = fmt.Sprintf("field %s must be at least %s characters long", field, err.Param())
		case "max":
			errMsgs[field] = fmt.Sprintf("field %s must be at most %s characters long", field, err.Param())
		default:
			errMsgs[field] = fmt.Sprintf("field %s is not valid", field)
		}
	}

	return Response{
		Status:           StatusError,
		Error:            "validation failed",
		ValidationErrors: errMsgs,
	}
}
