package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/rajatgoel/ratchet/internal/storage"
)

func TestDBConnection(t *testing.T) {
	ctx := context.Background()
	postgresContainer, err := postgres.Run(ctx, "postgres:latest", postgres.BasicWaitStrategies())
	require.NoError(t, err)

	_, err = storage.NewDBConnectionWithURL(ctx, postgresContainer.MustConnectionString(ctx, "sslmode=disable"))
	require.NoError(t, err)
}
