package binder

import (
	"reflect"
	"testing"

	ut "github.com/go-playground/universal-translator"
	"github.com/stretchr/testify/assert"
)

type mockFieldError struct {
	tag   string
	field string
	param string
	kind  reflect.Kind
}

func (e *mockFieldError) Error() string           { return "Mock Field Error" }
func (e *mockFieldError) Tag() string             { return e.tag }
func (e *mockFieldError) ActualTag() string       { return e.tag }
func (e *mockFieldError) Namespace() string       { return "" }
func (e *mockFieldError) StructNamespace() string { return "" }
func (e *mockFieldError) Field() string           { return e.field }
func (e *mockFieldError) StructField() string     { return "" }
func (e *mockFieldError) Value() interface{}      { return "" }
func (e *mockFieldError) Param() string           { return e.param }
func (e *mockFieldError) Kind() reflect.Kind {
	if e.kind == 0 {
		return reflect.String
	}
	return e.kind
}
func (e *mockFieldError) Type() reflect.Type               { return reflect.TypeOf("") }
func (e *mockFieldError) Translate(_ ut.Translator) string { return "" }

func TestFormatValidationError(t *testing.T) {
	cases := []struct {
		tag   string
		param string
		kind  reflect.Kind
		msg   string
	}{
		{email, "", 0, `"multi_word" is not a valid email`},
		{gt, "0", 0, `"multi_word" must be greater than 0`},
		// String min/max
		{mx, "20", reflect.String, `"multi_word" length must be less than or equal to 20 characters`},
		{mx, "1", reflect.String, `"multi_word" length must be less than or equal to 1 character`},
		{mn, "20", reflect.String, `"multi_word" length must be greater than or equal to 20 characters`},
		{mn, "1", reflect.String, `"multi_word" length must be greater than or equal to 1 character`},
		// Numeric min/max
		{mx, "50", reflect.Int, `"multi_word" must be less than or equal to 50`},
		{mx, "100", reflect.Int64, `"multi_word" must be less than or equal to 100`},
		{mx, "1", reflect.Uint, `"multi_word" must be less than or equal to 1`},
		{mn, "1", reflect.Int, `"multi_word" must be greater than or equal to 1`},
		{mn, "0", reflect.Float64, `"multi_word" must be greater than or equal to 0`},
		// Slice min/max
		{mx, "5", reflect.Slice, `"multi_word" length must be less than or equal to 5 elements`},
		{mx, "1", reflect.Slice, `"multi_word" length must be less than or equal to 1 element`},
		{mn, "2", reflect.Slice, `"multi_word" length must be greater than or equal to 2 elements`},
		{mn, "1", reflect.Slice, `"multi_word" length must be greater than or equal to 1 element`},
		// Other
		{ne, "20", 0, `"multi_word" can't be "20"`},
		{oneof, "one two three", 0, `"multi_word" must be one of the following: "one", "two", "three"`},
		{required, "", 0, `"multi_word" is required`},
		{"foo", "", 0, "NOT IMPLEMENTED YET"},
	}

	for _, tt := range cases {
		err := mockFieldError{tag: tt.tag, field: "multi_word", param: tt.param, kind: tt.kind}
		msg := formatValidationError(&err)
		assert.Equal(t, tt.msg, msg)
	}
}
