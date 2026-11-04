package dbmigrate

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"base-admin/server/internal/platform/config"
	"base-admin/server/internal/platform/database"
)

// migrationNames lists the migration files of a dialect.
func migrationNames(t *testing.T, tr Track, d database.Dialect) []string {
	t.Helper()
	files, err := tr.FilesFor(d)
	require.NoError(t, err)
	names, err := fs.Glob(files, "*.sql")
	require.NoError(t, err)
	return names
}

// The scaffold keeps one migration per version for each dialect, with the
// same file names, so both databases reach the same schema version.
func TestScaffoldMigrationsMatchAcrossDialects(t *testing.T) {
	pg := migrationNames(t, BaseTrack, database.Postgres)
	my := migrationNames(t, BaseTrack, database.MySQL)
	require.NotEmpty(t, pg)
	assert.Equal(t, pg, my, "every PostgreSQL migration needs its MySQL version in migrations/mysql")

	pgLatest, err := BaseTrack.Latest(database.Postgres)
	require.NoError(t, err)
	myLatest, err := BaseTrack.Latest(database.MySQL)
	require.NoError(t, err)
	assert.Equal(t, pgLatest, myLatest)
}

func TestTracksHaveFilesForEveryDialect(t *testing.T) {
	for _, tr := range TracksFor(config.DatabaseConfig{}) {
		for _, d := range []database.Dialect{database.Postgres, database.MySQL} {
			_, err := tr.FilesFor(d)
			assert.NoError(t, err, "%s/%s", tr.Name, d)
		}
	}
	_, err := Track{Name: "x"}.FilesFor(database.MySQL)
	assert.Error(t, err)
}
