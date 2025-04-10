package binder

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type params struct {
	Hello string `json:"hello" mod:"trim" validate:"max=9"`
	Omit  string `json:"-"`
}

var (
	goodJSON             = `{"hello":" world "}`
	unknownFieldsErrJSON = `{"hello":"world","foo":"bar"}`
	typeErrJSON          = `{"hello":123}`
	validationErrJSON    = `{"hello":"0123456789"}`
)

func TestNew(t *testing.T) {
	b, err := New()
	require.NoError(t, err)
	assert.NotNil(t, b)

	t.Run("only allows application/json and application/x-www-form-urlencoded", func(tt *testing.T) {
		c := newContext(goodJSON, echo.MIMEApplicationXML)
		p := params{}
		err = b.Bind(&p, c)
		assert.Contains(tt, err.Error(), "Unsupported Media Type")
	})

	t.Run("disallows unknown fields", func(tt *testing.T) {
		c := newContext(unknownFieldsErrJSON, echo.MIMEApplicationJSON)
		p := params{}
		err = b.Bind(&p, c)
		assert.Contains(tt, err.Error(), `Unknown Parameter "foo"`)
	})

	t.Run("returns a good message for type errors", func(tt *testing.T) {
		c := newContext(typeErrJSON, echo.MIMEApplicationJSON)
		p := params{}
		err = b.Bind(&p, c)
		assert.Contains(tt, err.Error(), `"hello" should be of type string`)
	})

	t.Run("use mod tag to modify params", func(tt *testing.T) {
		c := newContext(goodJSON, echo.MIMEApplicationJSON)
		p := params{}
		err = b.Bind(&p, c)
		require.NoError(tt, err)
		assert.Equal(tt, "world", p.Hello)
	})

	t.Run("use validate tag to validate params", func(tt *testing.T) {
		c := newContext(validationErrJSON, echo.MIMEApplicationJSON)
		p := params{}
		err = b.Bind(&p, c)
		assert.Contains(tt, err.Error(), "length must be less than or equal to 9 characters")
	})
}

func newContext(payload, mime string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(echo.POST, "/", strings.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, mime)
	rr := httptest.NewRecorder()
	return e.NewContext(req, rr)
}
