package data

import (
	"context"
	"github.com/cryptopunkscc/astrald/mod/admin"
	"github.com/cryptopunkscc/astrald/mod/data"
	"github.com/cryptopunkscc/astrald/node/modules"
)

func (mod *Module) Prepare(ctx context.Context) error {
	// inject admin command
	if adm, err := modules.Load[admin.Module](mod.node, admin.ModuleName); err == nil {
		adm.AddCommand(data.ModuleName, NewAdmin(mod))
	}

	return nil
}
