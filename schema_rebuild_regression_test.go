package sqlite

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSQLiteRenameDefinitionKeepsExternalIdentifiers(t *testing.T) {
	renames := map[string]string{"id": "child_id"}
	require.Equal(t, `"child_id" id constraint id check ("child_id">0) references parent(id)`, sqliteRenameColumnDefinition(`id id constraint id check (id>0) references parent(id)`, "child", "child", renames))
	require.Equal(t, `parent_id integer references "renamed"("child_id") on delete set null`, sqliteRenameColumnDefinition(`parent_id integer references child(id) on delete set null`, "child", "renamed", renames))
	require.Equal(t, `constraint id foreign key("child_id") references parent(id)`, sqliteRenameForeign(`constraint id foreign key(id) references parent(id)`, "child", "child", renames))
}
