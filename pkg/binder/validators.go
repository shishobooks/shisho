package binder

import (
	"context"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/go-playground/mold/v4"
	"github.com/go-playground/validator/v10"
	"golang.org/x/text/unicode/norm"
)

var (
	dateRE = regexp.MustCompile(`^\d{4}-(0[0-9]|1[0-2])-(0[0-9]|1[0-9]|2[0-9]|3[0-1])$`)
)

// dateValidator ensures the value is a real calendar day as YYYY-MM-DD, so
// not February 30, or the empty string. The reason the empty string is allowed is that this validator can be
// used to clear out values. However, this is only useful in that case, so if
// you're using this validator but want the value to be required, add a `ne=` to
// the validate tag so that the empty string is disallowed.
func dateValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true
	}
	if !dateRE.MatchString(value) {
		return false
	}
	_, err := time.Parse(time.DateOnly, value)
	return err == nil
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

// usernameSymbols are the characters a username may hold besides letters and
// numbers. "@" and "." keep email-style usernames working.
const usernameSymbols = "._-@"

// usernameValidator ensures a username is only letters (with their marks)
// and numbers, in any script, and usernameSymbols. A colon would break HTTP Basic sign-in (OPDS,
// KOReader), which splits on the first one, and spaces or invisible
// characters would let one username pass for another. Length is checked
// separately. Sign-in doesn't use it, so an older username that breaks the
// rule still signs in.
func usernameValidator(fl validator.FieldLevel) bool {
	for _, r := range fl.Field().String() {
		// Marks are the vowel signs and accents many scripts write letters
		// with, such as Devanagari and Thai.
		if !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsNumber(r) && !strings.ContainsRune(usernameSymbols, r) {
			return false
		}
	}
	return true
}

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
