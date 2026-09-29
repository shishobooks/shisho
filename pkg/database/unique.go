package database

import "strings"

// IsUniqueViolation reports whether err came from a SQLite UNIQUE constraint.
func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint")
}

// RetrieveOnUniqueViolation finishes a find-or-create. When the insert failed
// on a UNIQUE constraint because another request inserted the same row between
// the lookup and the insert, it returns what retrieve finds instead. Otherwise
// it returns created, or createErr when the insert failed for another reason.
func RetrieveOnUniqueViolation[T any](created T, createErr error, retrieve func() (T, error)) (T, error) {
	if createErr == nil {
		return created, nil
	}
	if IsUniqueViolation(createErr) {
		return retrieve()
	}
	var zero T
	return zero, createErr
}
