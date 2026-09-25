package worker

import (
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deleting a shared Publisher leaves each affected File with a protected
// manual empty slot. An ordinary Scan must not recreate or reattach it from
// plugin or embedded metadata, while Refresh all metadata and Reset to file
// metadata may repopulate it under their existing rules.

func deleteFilePublisher(t *testing.T, tc *testContext, file *models.File) string {
	t.Helper()
	require.NotNil(t, file.Publisher, "precondition: the File has a Publisher")
	name := file.Publisher.Name
	require.NoError(t, publishers.NewService(tc.db).DeletePublisher(tc.ctx, file.Publisher.ID))
	return name
}

func requireManualEmptyPublisher(t *testing.T, file *models.File) {
	t.Helper()
	require.Nil(t, file.PublisherID)
	require.NotNil(t, file.PublisherSource)
	require.Equal(t, models.DataSourceManual, *file.PublisherSource)
}

func assertPublisherNotRecreated(t *testing.T, tc *testContext, libraryID int, name string) {
	t.Helper()
	_, err := publishers.NewService(tc.db).RetrievePublisher(tc.ctx, publishers.RetrievePublisherOptions{Name: &name, LibraryID: &libraryID})
	assert.Error(t, err, "the deleted Publisher %q must not be recreated", name)
}

func newPublisherDeleteEPUBFixture(t *testing.T) *identifyScanFixture {
	t.Helper()
	tc, bookDir := newIdentifyScalarClearContext(t)
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:     "Embedded Title",
		Authors:   []string{"Embedded Author"},
		Publisher: "Embedded Publisher",
	})
	require.NoError(t, tc.runScan())
	return &identifyScanFixture{tc: tc}
}

// The reproduction from the issue: a plugin-sourced Publisher whose file also
// embeds a publisher. The auto-enricher proposes the same Publisher again on
// every Scan.
func TestDeletePublisher_ThenOrdinaryScan_PluginSource(t *testing.T) {
	t.Parallel()

	f := newIdentifyScanFixture(t)
	_, file := f.retrieve(t)
	require.Equal(t, "Plugin Publisher", file.Publisher.Name, "precondition: the auto-enricher wins the first Scan")
	require.Equal(t, "plugin:test/auto-enricher", *file.PublisherSource)

	name := deleteFilePublisher(t, f.tc, file)
	_, file = f.retrieve(t)
	requireManualEmptyPublisher(t, file)

	f.ordinaryScan(t, file.ID)

	_, file = f.retrieve(t)
	requireManualEmptyPublisher(t, file)
	assertPublisherNotRecreated(t, f.tc, file.LibraryID, name)
}

// A Publisher that came from embedded metadata ties with a rescan of the same
// file, so without the manual stamp an ordinary Scan would recreate it.
func TestDeletePublisher_ThenOrdinaryScan_EmbeddedSource(t *testing.T) {
	t.Parallel()

	f := newPublisherDeleteEPUBFixture(t)
	_, file := f.retrieve(t)
	require.Equal(t, "Embedded Publisher", file.Publisher.Name, "precondition: the Scan reads embedded metadata")
	require.Equal(t, models.DataSourceEPUBMetadata, *file.PublisherSource)

	name := deleteFilePublisher(t, f.tc, file)
	f.ordinaryScan(t, file.ID)

	_, file = f.retrieve(t)
	requireManualEmptyPublisher(t, file)
	assertPublisherNotRecreated(t, f.tc, file.LibraryID, name)
}

func TestDeletePublisher_ThenRefreshAllMetadata_Repopulates(t *testing.T) {
	t.Parallel()

	f := newIdentifyScanFixture(t)
	_, file := f.retrieve(t)
	deleteFilePublisher(t, f.tc, file)

	_, err := f.tc.worker.scanInternal(f.tc.ctx, ScanOptions{FileID: file.ID, ForceRefresh: true}, nil)
	require.NoError(t, err)

	_, file = f.retrieve(t)
	require.NotNil(t, file.Publisher, "Refresh all metadata may repopulate a deleted Publisher")
	assert.Equal(t, "Plugin Publisher", file.Publisher.Name)
	require.NotNil(t, file.PublisherSource)
	assert.Equal(t, "plugin:test/auto-enricher", *file.PublisherSource)
}

func TestDeletePublisher_ThenResetToFileMetadata_Repopulates(t *testing.T) {
	t.Parallel()

	f := newPublisherDeleteEPUBFixture(t)
	_, file := f.retrieve(t)
	deleteFilePublisher(t, f.tc, file)

	_, err := f.tc.worker.scanInternal(f.tc.ctx, ScanOptions{FileID: file.ID, ForceRefresh: true, SkipPlugins: true, Reset: true}, nil)
	require.NoError(t, err)

	_, file = f.retrieve(t)
	require.NotNil(t, file.Publisher, "Reset to file metadata may repopulate a deleted Publisher")
	assert.Equal(t, "Embedded Publisher", file.Publisher.Name)
	require.NotNil(t, file.PublisherSource)
	assert.Equal(t, models.DataSourceEPUBMetadata, *file.PublisherSource)
}
