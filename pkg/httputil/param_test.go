package httputil

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParamID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{name: "positive", value: "42", want: 42},
		{name: "one", value: "1", want: 1},
		{name: "zero", value: "0", wantErr: true},
		{name: "negative", value: "-3", wantErr: true},
		{name: "not a number", value: "abc", wantErr: true},
		{name: "empty", value: "", wantErr: true},
		{name: "decimal", value: "1.5", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			c.SetParamNames("id")
			c.SetParamValues(tt.value)

			got, err := ParamID(c, "id", "Series")
			if !tt.wantErr {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
				return
			}
			require.ErrorIs(t, err, errcodes.NotFound("Series"))
			assert.Equal(t, "Series not found.", err.Error())
			assert.Zero(t, got)
		})
	}
}
