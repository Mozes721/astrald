package fwd

import (
	"context"
	"github.com/cryptopunkscc/astrald/auth/id"
	"github.com/cryptopunkscc/astrald/net"
	"github.com/cryptopunkscc/astrald/node/services"
	"strings"
)

var _ Server = &AstralServer{}

type AstralServer struct {
	*Module
	serviceName string
	identity    id.Identity
	target      net.Router
	service     *services.Service
}

func NewAstralServer(mod *Module, serviceName string, target net.Router) (*AstralServer, error) {
	var err error
	var identity = mod.node.Identity()
	var srv = &AstralServer{
		Module: mod,
		target: target,
	}

	if idx := strings.Index(serviceName, "@"); idx != -1 {
		name := serviceName[:idx]
		identity, err = mod.node.Resolver().Resolve(name)
		if err != nil {
			return nil, err
		}

		// fetch private key if we're calling as non-node identity
		if !identity.IsEqual(mod.node.Identity()) {
			keystore, err := mod.assets.KeyStore()
			if err != nil {
				return nil, err
			}

			identity, err = keystore.Find(identity)
			if err != nil {
				return nil, err
			}
		}

		serviceName = serviceName[idx+1:]
	}

	srv.identity = identity
	srv.serviceName = serviceName
	srv.service, err = srv.node.Services().Register(context.Background(), identity, serviceName, srv)
	if err != nil {
		return nil, err
	}

	return srv, nil
}

func (srv *AstralServer) Run(ctx context.Context) error {
	<-ctx.Done()
	srv.service.Close()
	return nil
}

func (srv *AstralServer) RouteQuery(ctx context.Context, query net.Query, caller net.SecureWriteCloser, hints net.Hints) (net.SecureWriteCloser, error) {
	dst, err := srv.target.RouteQuery(ctx, query, caller, hints)
	if err != nil {
		return nil, err
	}

	return net.NewSecurePipeWriter(dst, srv.identity), nil
}

func (srv *AstralServer) Target() net.Router {
	return srv.target
}

func (srv *AstralServer) String() string {
	return "astral://" + srv.serviceName
}