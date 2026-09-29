package review

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/require"
)

func TestDefault_HasExpectedFields(t *testing.T) {
	t.Parallel()
	d := Default()
	require.ElementsMatch(t, []string{"authors", "description", "cover", "genres"}, d.BookFields)
	require.ElementsMatch(t, []string{"narrators"}, d.AudioFields)
}

func TestLoad_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	svc := appsettings.NewService(db)

	c, err := Load(ctx, svc)
	require.NoError(t, err)
	require.Equal(t, Default(), c)
}

func TestLoad_ReadsSavedValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	svc := appsettings.NewService(db)

	want := Criteria{BookFields: []string{"authors"}, AudioFields: []string{}}
	require.NoError(t, Save(ctx, svc, want))

	got, err := Load(ctx, svc)
	require.NoError(t, err)
	require.Equal(t, want.BookFields, got.BookFields)
	require.Equal(t, want.AudioFields, got.AudioFields)
}

func TestValidate_RejectsUnknownField(t *testing.T) {
	t.Parallel()
	err := Validate(Criteria{BookFields: []string{"made_up_field"}})
	require.Error(t, err)
}

func TestValidate_RejectsAudioFieldInUniversal(t *testing.T) {
	t.Parallel()
	err := Validate(Criteria{BookFields: []string{"narrators"}})
	require.Error(t, err)
}

func TestValidate_AcceptsKnown(t *testing.T) {
	t.Parallel()
	err := Validate(Default())
	require.NoError(t, err)
}
