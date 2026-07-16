package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yuuyakim/menu-planner/backend/internal/db"
)

func TestMigrate_DSNが不正ならエラー(t *testing.T) {
	t.Parallel()

	err := db.Migrate("not-a-dsn", db.MigrateUp)
	require.Error(t, err)
}

func TestParseMigrateDirection(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    db.MigrateDirection
		wantErr bool
	}{
		"up":      {input: "up", want: db.MigrateUp},
		"down":    {input: "down", want: db.MigrateDown},
		"version": {input: "version", want: db.MigrateVersion},
		"空文字":     {input: "", wantErr: true},
		"未知の値":    {input: "sideways", wantErr: true},
		"大文字は不許可": {input: "UP", wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := db.ParseMigrateDirection(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMigrationFS_SQLが埋め込まれている(t *testing.T) {
	t.Parallel()

	entries, err := db.MigrationFiles()
	require.NoError(t, err)

	// up と down が対で存在すること
	assert.Contains(t, entries, "000001_create_menus.up.sql")
	assert.Contains(t, entries, "000001_create_menus.down.sql")
	assert.Zero(t, len(entries)%2, "up/down が対になっていること")
}
