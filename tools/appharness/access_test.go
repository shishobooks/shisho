package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestCheapPasswordHashStillChecksThePassword(t *testing.T) {
	t.Parallel()
	hash, err := cheapPasswordHash("harness-password")
	require.NoError(t, err)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("harness-password")))
	assert.Error(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong")))
	cost, err := bcrypt.Cost([]byte(hash))
	require.NoError(t, err)
	assert.Equal(t, bcrypt.MinCost, cost)
}
