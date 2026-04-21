package audnexus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_GetChapters_InvalidASIN(t *testing.T) {
	t.Parallel()
	svc := NewService(ServiceConfig{})

	cases := []string{
		"",
		"short",
		"TOO-LONG-ASIN",
		"1234567890X!", // non-alphanumeric
		"B0036UC2L",    // 9 chars
		"B0036UC2LOX",  // 11 chars
	}
	for _, asin := range cases {
		asin := asin
		t.Run(asin, func(t *testing.T) {
			t.Parallel()
			_, err := svc.GetChapters(context.Background(), asin)
			require.Error(t, err)
			e := AsAudnexusError(err)
			require.NotNil(t, e, "expected *Error")
			assert.Equal(t, ErrCodeInvalidASIN, e.Code)
		})
	}
}

func TestService_GetChapters_NormalizesASINToUppercase(t *testing.T) {
	t.Parallel()
	// Valid lowercase ASIN should pass validation (and would reach upstream,
	// which we're not testing here — but it must not return invalid_asin).
	svc := NewService(ServiceConfig{})
	_, err := svc.GetChapters(context.Background(), "b0036uc2lo")
	if e := AsAudnexusError(err); e != nil {
		assert.NotEqual(t, ErrCodeInvalidASIN, e.Code, "lowercase ASIN should normalize to valid")
	}
}
