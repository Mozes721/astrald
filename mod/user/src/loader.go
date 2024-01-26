package user

import (
	"github.com/cryptopunkscc/astrald/log"
	"github.com/cryptopunkscc/astrald/mod/user"
	"github.com/cryptopunkscc/astrald/node/assets"
	"github.com/cryptopunkscc/astrald/node/modules"
)

type Loader struct{}

func (Loader) Load(node modules.Node, assets assets.Assets, log *log.Logger) (modules.Module, error) {
	var err error
	var mod = &Module{
		node:   node,
		log:    log,
		assets: assets,
	}

	mod.profileService = &ProfileService{Module: mod}
	mod.notifyService = &NotifyService{Module: mod}

	mod.db = assets.Database()

	err = mod.db.AutoMigrate(&dbIdentity{})
	if err != nil {
		return nil, err
	}

	_ = assets.LoadYAML(user.ModuleName, &mod.config)

	return mod, err
}

func init() {
	if err := modules.RegisterModule(user.ModuleName, Loader{}); err != nil {
		panic(err)
	}
}
