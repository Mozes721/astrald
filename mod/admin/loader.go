package admin

import (
	"github.com/cryptopunkscc/astrald/node"
)

const ModuleName = "admin"

type Loader struct{}

func (Loader) Load(node *node.Node) (node.Module, error) {
	mod := &Admin{node: node}

	return mod, nil
}

func (Loader) Name() string {
	return ModuleName
}