package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("APP_SERVER_HTTP", "127.0.0.1:9999")

	cfg, err := load("dev")
	require.NoError(t, err)

	assert.Equal(t, "receipt-autoitemize", cfg.App.Name)
	assert.Equal(t, "dev", cfg.App.Env)
	assert.Equal(t, "127.0.0.1:9999", cfg.Server.HTTP)
	assert.Equal(t, "127.0.0.1:8082", cfg.Server.GRPC)
	assert.Equal(t, "data/app.db", cfg.Database.Path)
	assert.Equal(t, "data/uploads", cfg.Storage.UploadDir)
	assert.Equal(t, int64(10<<20), cfg.Storage.MaxUploadBytes)
	assert.Equal(t, "stub", cfg.OCR.Provider)
	assert.Equal(t, "fixtures/task-a", cfg.OCR.FixturesDir)
}
