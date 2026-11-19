package binder

import (
	"context"
	"reflect"

	"github.com/go-playground/mold/v4"
	"golang.org/x/text/unicode/norm"
)

// nfcModifier puts a string in Unicode composed form (NFC), so text that
// looks the same is stored the same way: "é" typed as one character or as
// "e" plus a combining accent. Usernames use it, so two that look alike
// can't both register.
func nfcModifier(_ context.Context, fl mold.FieldLevel) error {
	if fl.Field().Kind() == reflect.String {
		fl.Field().SetString(norm.NFC.String(fl.Field().String()))
	}
	return nil
}
