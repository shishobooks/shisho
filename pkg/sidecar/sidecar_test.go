package sidecar

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestFileSidecarFromModel_Name(t *testing.T) {
	name := "Custom File Name"
	file := &models.File{
		Name: &name,
	}

	sidecar := FileSidecarFromModel(file)

	assert.NotNil(t, sidecar.Name)
	assert.Equal(t, "Custom File Name", *sidecar.Name)
}

func TestFileSidecarFromModel_NilName(t *testing.T) {
	file := &models.File{
		Name: nil,
	}

	sidecar := FileSidecarFromModel(file)

	assert.Nil(t, sidecar.Name)
}
