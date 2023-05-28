package modules

import (
	"context"
	"github.com/cryptopunkscc/astrald/auth/id"
	"github.com/cryptopunkscc/astrald/node/event"
	"github.com/cryptopunkscc/astrald/node/infra"
	"github.com/cryptopunkscc/astrald/node/link"
	"github.com/cryptopunkscc/astrald/node/network"
	"github.com/cryptopunkscc/astrald/node/resolver"
	"github.com/cryptopunkscc/astrald/node/services"
	"github.com/cryptopunkscc/astrald/node/tracker"
)

// Node is a subset of node.Node that's exposed to modules
type Node interface {
	Identity() id.Identity
	Alias() string
	Query(ctx context.Context, remoteID id.Identity, query string) (link.BasicConn, error)
	Events() *event.Queue
	Infra() infra.Infra
	Network() network.Network
	Tracker() tracker.Tracker
	Services() services.Services
	Modules() Modules
	Resolver() resolver.Resolver
}