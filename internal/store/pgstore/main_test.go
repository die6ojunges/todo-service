package pgstore_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// postgresImage matches compose.yaml, so tests run on the same server version as local dev.
const postgresImage = "postgres:17.6-alpine"

// testDatabaseURL points at the disposable PostgreSQL started by TestMain.
var testDatabaseURL string

func TestMain(m *testing.M) {
	os.Exit(runWithPostgres(m))
}

// runWithPostgres starts one throwaway PostgreSQL for the whole package (startup is the slow part);
// each test still gets its own schema via isolatedPool, so tests stay independent. It fails instead
// of skipping when Docker is missing, so a green run always means the store was really tested.
func runWithPostgres(m *testing.M) int {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, postgresImage, tcpostgres.WithDatabase("todo"),
		tcpostgres.WithUsername("todo"), tcpostgres.WithPassword("todo"), tcpostgres.BasicWaitStrategies())
	defer func() { _ = testcontainers.TerminateContainer(container) }()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start %s container (is Docker running?): %v\n", postgresImage, err)
		return 1
	}
	if testDatabaseURL, err = container.ConnectionString(ctx, "sslmode=disable"); err != nil {
		fmt.Fprintf(os.Stderr, "read container connection string: %v\n", err)
		return 1
	}
	return m.Run()
}
