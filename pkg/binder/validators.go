package binder

import (
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/go-playground/validator/v10"
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

// usernameValidator ensures a username is only letters and numbers, in any
// script, the marks that letters are written with (the vowel signs and
// accents of scripts such as Devanagari and Thai), and usernameSymbols. A
// colon would break HTTP Basic sign-in (OPDS, KOReader), which splits on the
// first one, and spaces or invisible characters would let one username pass
// for another, so a mark must follow a letter and invisible ones are refused.
// Length is checked separately. Sign-in doesn't use it, so an older username
// that breaks the rule still signs in.
func usernameValidator(fl validator.FieldLevel) bool {
	afterLetter := false
	for _, r := range fl.Field().String() {
		if invisible(r) {
			return false
		}
		switch {
		case unicode.IsLetter(r):
			afterLetter = true
		case unicode.IsMark(r):
			if !afterLetter {
				return false
			}
		case unicode.IsNumber(r), strings.ContainsRune(usernameSymbols, r):
			afterLetter = false
		default:
			return false
		}
	}
	return true
}

// invisible reports whether r renders as nothing though Unicode files it
// under letters or marks: variation selectors, the combining grapheme
// joiner, and fillers such as the Hangul ones.
func invisible(r rune) bool {
	return unicode.Is(unicode.Variation_Selector, r) ||
		unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r)
}
