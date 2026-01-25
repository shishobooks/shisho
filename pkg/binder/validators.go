package binder

import (
	"net/url"
	"regexp"

	"github.com/go-playground/validator/v10"
)

var (
	dateRE = regexp.MustCompile(`^\d{4}-(0[0-9]|1[0-2])-(0[0-9]|1[0-9]|2[0-9]|3[0-1])$`)
)

// dateValidator ensures the value matches the format YYYY-MM-DD or the empty
// string. The reason the empty string is allowed is that this validator can be
// used to clear out values. However, this is only useful in that case, so if
// you're using this validator but want the value to be required, add a `ne=` to
// the validate tag so that the empty string is disallowed.
func dateValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true
	}
	return dateRE.MatchString(value)
}

// urlValidator ensures the value is a valid URL or the empty string. The empty
// string is allowed so that this validator can be used to clear out values. If
// you want to enforce a non-empty URL, add a `required` tag.
func urlValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true
	}
	u, err := url.Parse(value)
	return err == nil && u.Scheme != "" && u.Host != ""
}
