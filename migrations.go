package todonotificator

import "embed"

//go:embed migrations/*.sql
var MigrationsFS embed.FS
