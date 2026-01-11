package binder

import (
	"fmt"
	"reflect"
	"strings"
	timepkg "time"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/schema"
	"github.com/segmentio/encoding/json"
)

const (
	date     = "date"
	email    = "email"
	gt       = "gt"
	gte      = "gte"
	gtfield  = "gtfield"
	ltfield  = "ltfield"
	mx       = "max"
	mn       = "min"
	ne       = "ne"
	oneof    = "oneof"
	required = "required"
)

var (
	timeType = reflect.TypeOf(timepkg.Time{})
)

func formatUnmarshalTypeError(err *json.UnmarshalTypeError) string {
	// FIXME: this doesn't work well for incorrect map values, e.g. it will say
	// `"metadata" should be a string instead of a object` if you pass in
	// `{"metadata":{"foo":{"bar":"baz"}}}`.
	return fmt.Sprintf("%q should be of type %s", strings.Trim(err.Field, "."), err.Type)
}

func formatSchemaConversionError(err schema.ConversionError) string {
	return fmt.Sprintf("%q should be of type %s", err.Key, err.Type)
}

func formatValidationError(err validator.FieldError) string {
	field := err.Field()

	switch err.Tag() {
	case date:
		return fmt.Sprintf("%q should be in the format of YYYY-MM-DD", field)
	case email:
		return fmt.Sprintf("%q is not a valid email", field)
	case gt:
		v := err.Param()
		if v == "" && err.Type() == timeType {
			v = "now"
		}
		return fmt.Sprintf("%q must be greater than %s", field, v)
	case gte:
		v := err.Param()
		if v == "" && err.Type() == timeType {
			v = "now"
		}
		return fmt.Sprintf("%q must be greater than or equal to %s", field, v)
	case gtfield:
		// FIXME: err.Param() will return the struct field, not the JSON version
		// e.g. EndTime, not end_time
		return fmt.Sprintf("%q must be greater than %s", field, err.Param())
	case ltfield:
		// FIXME: err.Param() will return the struct field, not the JSON version
		// e.g. EndTime, not end_time
		return fmt.Sprintf("%q must be less than %s", field, err.Param())
	case mx:
		//exhaustive:ignore
		switch err.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			return fmt.Sprintf("%q must be less than or equal to %s", field, err.Param())
		case reflect.Slice:
			resource := "element"
			if err.Param() != "1" {
				resource += "s"
			}
			return fmt.Sprintf("%q length must be less than or equal to %s %s", field, err.Param(), resource)
		default:
			resource := "character"
			if err.Param() != "1" {
				resource += "s"
			}
			return fmt.Sprintf("%q length must be less than or equal to %s %s", field, err.Param(), resource)
		}
	case mn:
		//exhaustive:ignore
		switch err.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			return fmt.Sprintf("%q must be greater than or equal to %s", field, err.Param())
		case reflect.Slice:
			resource := "element"
			if err.Param() != "1" {
				resource += "s"
			}
			return fmt.Sprintf("%q length must be greater than or equal to %s %s", field, err.Param(), resource)
		default:
			resource := "character"
			if err.Param() != "1" {
				resource += "s"
			}
			return fmt.Sprintf("%q length must be greater than or equal to %s %s", field, err.Param(), resource)
		}
	case ne:
		return fmt.Sprintf("%q can't be %q", field, err.Param())
	case oneof:
		valids := []string{}
		for _, p := range strings.Fields(err.Param()) {
			valids = append(valids, fmt.Sprintf("%q", p))
		}
		return fmt.Sprintf("%q must be one of the following: %s", field, strings.Join(valids, ", "))
	case required:
		return fmt.Sprintf("%q is required", field)
	default:
		// these print statements aid in determining how to construct
		// the error messages for validation functions that haven't been
		// implemented yet
		fmt.Println("actual tag", err.ActualTag())
		fmt.Println("field", field)
		fmt.Println("param", err.Param())
		fmt.Println("struct field", err.StructField())
		fmt.Println("struct namspace", err.StructNamespace())
		fmt.Println("tag", err.Tag())
		fmt.Println("kind", err.Kind())
		fmt.Println("type", err.Type())

		return "NOT IMPLEMENTED YET"
	}
}
