//go:build ruleguard

// Package gorules holds the go-ruleguard rules that golangci-lint loads
// through gocritic's ruleguard check (see .golangci.yml). The build tag keeps
// the file out of normal builds and tests; go mod tidy still sees the dsl
// import, so the dependency stays in go.mod.
package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

// errorText flags matching on an error's message. Messages are not API: a
// wrapped or reworded error silently stops matching.
func errorText(m dsl.Matcher) {
	m.Match(
		`$err.Error() == $_`,
		`$_ == $err.Error()`,
		`$err.Error() != $_`,
		`$_ != $err.Error()`,
		`strings.Contains($err.Error(), $_)`,
		`strings.HasPrefix($err.Error(), $_)`,
		`strings.HasSuffix($err.Error(), $_)`,
	).
		Where(m["err"].Type.Implements("error")).
		Report(`match errors with errors.Is or errors.As against a sentinel, not by message text`)
}

// pathIDParse flags parsing a numeric path ID by hand. httputil.ParamID
// returns errcodes.NotFound for a non-numeric or non-positive ID, matching
// RequireLibraryAccess. Only params named like an ID match, so page numbers
// and other numeric params stay free to return a ValidationError.
func pathIDParse(m dsl.Matcher) {
	m.Import("github.com/labstack/echo/v4")
	m.Match(
		`strconv.Atoi($c.Param($name))`,
		`strconv.ParseInt($c.Param($name), $*_)`,
		`strconv.ParseUint($c.Param($name), $*_)`,
	).
		Where(m["c"].Type.Is("echo.Context") && m["name"].Text.Matches(`(?i)^"(id|.*id)"$`)).
		Report(`use httputil.ParamID, which returns NotFound for a non-numeric path ID`)
}

// jsonMapResponse flags map-typed JSON responses. tygo generates frontend
// types only from named structs (ADR 0004), so a map leaves the shape untyped.
func jsonMapResponse(m dsl.Matcher) {
	m.Import("github.com/labstack/echo/v4")
	m.Match(`$c.JSON($_, $v)`).
		Where(m["c"].Type.Is("echo.Context") && m["v"].Type.Underlying().Is(`map[$_]$_`)).
		Report(`return a named response struct from the package's types.go, not a map`)
}
