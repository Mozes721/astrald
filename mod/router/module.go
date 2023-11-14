package router

import (
	"context"
	"errors"
	"fmt"
	"github.com/cryptopunkscc/astrald/auth/id"
	"github.com/cryptopunkscc/astrald/cslq"
	"github.com/cryptopunkscc/astrald/log"
	"github.com/cryptopunkscc/astrald/mod/router/proto"
	"github.com/cryptopunkscc/astrald/net"
	"github.com/cryptopunkscc/astrald/node"
	"github.com/cryptopunkscc/astrald/node/assets"
	"github.com/cryptopunkscc/astrald/streams"
	"github.com/cryptopunkscc/astrald/tasks"
	"time"
)

type Module struct {
	node   node.Node
	keys   assets.KeyStore
	log    *log.Logger
	config Config
	ctx    context.Context
}

func (mod *Module) Run(ctx context.Context) error {
	mod.ctx = ctx

	// register as a router
	if coreRouter, ok := mod.node.Router().(*node.CoreRouter); ok {
		coreRouter.Routers.AddRouter(mod)
	} else {
		return errors.New("unsupported router type")
	}

	return tasks.Group(
		&RouterService{Module: mod},
		&RerouteService{Module: mod},
	).Run(ctx)
}

func (mod *Module) RouteQuery(ctx context.Context, query net.Query, caller net.SecureWriteCloser, hints net.Hints) (net.SecureWriteCloser, error) {
	if mod.isLocal(query.Target()) {
		return net.RouteNotFound(mod)
	}

	if query.Caller().IsEqual(mod.node.Identity()) {
		return net.RouteNotFound(mod)
	}

	if mod.node.Network().Links().ByRemoteIdentity(query.Target()).Count() > 0 {
		return mod.RouteVia(ctx, query.Target(), query, caller, hints)
	}

	return net.RouteNotFound(mod)
}

func (mod *Module) RouteVia(
	ctx context.Context,
	routerIdentity id.Identity,
	query net.Query,
	caller net.SecureWriteCloser,
	hints net.Hints,
) (target net.SecureWriteCloser, err error) {
	// TODO: remove this once we have persistent certificates
	if query.Caller().PrivateKey() == nil {
		return net.RouteNotFound(mod, errors.New("caller private key missing"))
	}

	// prepare query parameters
	var queryParams = &proto.QueryParams{
		Target: query.Target(),
		Query:  query.Query(),
		Nonce:  uint64(query.Nonce()),
	}

	// attach a caller certificate if necessary
	if !query.Caller().IsEqual(mod.node.Identity()) {
		// TODO: fetch certificate from db instead of signing a new one every time
		var cert = NewRouterCert(query.Caller(), mod.node.Identity(), time.Now().Add(time.Minute))
		queryParams.Cert, err = cslq.Marshal(cert)
		if err != nil {
			return net.RouteNotFound(mod, err)
		}
	}

	// open a router session
	routerConn, err := net.Route(ctx, mod.node.Router(), net.NewQuery(mod.node.Identity(), routerIdentity, RouterServiceName))
	if err != nil {
		return nil, err
	}
	defer routerConn.Close()
	var router = proto.New(routerConn)

	// query the router
	response, err := router.Query(queryParams)
	switch {
	case errors.Is(err, proto.ErrRejected):
		return net.Reject()
	case err != nil:
		return net.RouteNotFound(mod, err)
	}

	var targetIM = NewIdentityMachine(routerIdentity)

	// apply target certificate
	if len(response.Cert) > 0 {
		if err = targetIM.Apply(response.Cert); err != nil {
			return net.RouteNotFound(mod, err)
		}
	}

	// verify target identity
	if !targetIM.Identity().IsEqual(query.Target()) {
		return net.RouteNotFound(mod, errors.New("target identity mismatch"))
	}

	// route through the proxy service
	var proxyQuery = net.NewQueryNonce(mod.node.Identity(), routerIdentity, response.ProxyService, query.Nonce())
	if !caller.Identity().IsEqual(mod.node.Identity()) {
		caller = NewIdentityTranslation(caller, mod.node.Identity())
	}
	proxy, err := mod.node.Router().RouteQuery(ctx, proxyQuery, caller, net.DefaultHints().SetDontMonitor().SetAllowRedirect())
	if err != nil {
		return net.RouteNotFound(mod, err)
	}

	if !proxy.Identity().IsEqual(query.Target()) {
		proxy = NewIdentityTranslation(proxy, query.Target())
	}

	return proxy, nil
}

func (mod *Module) Reroute(nonce net.Nonce, router net.Router) error {
	conn := mod.findConnByNonce(nonce)
	if conn == nil {
		return errors.New("conn not found")
	}

	routerIdentity := mod.getRouter(conn.Target())
	if routerIdentity.IsZero() {
		return errors.New("cannot establish router identity")
	}

	serviceQuery := net.NewQuery(mod.node.Identity(), routerIdentity, RerouteServiceName)
	serviceConn, err := net.Route(mod.ctx, router, serviceQuery)
	if err != nil {
		return err
	}

	if err := cslq.Encode(serviceConn, "q", nonce); err != nil {
		return err
	}

	var errCode int
	cslq.Decode(serviceConn, "c", &errCode)
	if errCode != 0 {
		return fmt.Errorf("error code %d", errCode)
	}

	switcher, err := mod.insertSwitcherAfter(net.RootSource(conn.Caller()))
	if err != nil {
		return err
	}

	newRoot, ok := net.RootSource(serviceConn).(net.OutputGetSetter)
	if !ok {
		return errors.New("newroot not an OutputGetSetter")
	}

	debris := newRoot.Output()
	newRoot.SetOutput(switcher.NextWriter)

	newOutput := mod.yankFinalOutput(serviceConn)
	oldOutput := net.FinalOutput(conn.Target())
	if err := mod.replaceOutput(oldOutput, newOutput); err != nil {
		return err
	}

	switcher.AfterSwitch = func() {
		debris.Close()
		serviceConn.Close()
	}

	return oldOutput.Close()
}

func (mod *Module) yankFinalOutput(chain any) net.SecureWriteCloser {
	final := net.FinalOutput(chain)

	s, ok := final.(net.SourceGetSetter)
	if !ok {
		return nil
	}

	prev, ok := s.Source().(net.OutputGetSetter)
	if !ok {
		return nil
	}

	prev.SetOutput(net.NewSecurePipeWriter(streams.NilWriteCloser{}, id.Identity{}))
	s.SetSource(nil)

	return final
}

func (mod *Module) replaceOutput(old, new net.SecureWriteCloser) error {
	var prev net.OutputSetter

	if old == nil {
		panic("old is nil")
	}
	if new == nil {
		panic("new is nil")
	}

	s, ok := old.(net.SourceGetter)
	if !ok {
		return errors.New("old output is not a SourceGetter")
	}

	prev, ok = s.Source().(net.OutputSetter)
	if !ok {
		return errors.New("source is not an OutputSetter")
	}

	return prev.SetOutput(new)
}

func (mod *Module) insertSwitcherAfter(item any) (*SwitchWriter, error) {
	i, ok := item.(net.OutputGetSetter)
	if !ok {
		return nil, fmt.Errorf("argument not an OutputGetSetter")
	}

	switcher := NewSwitchWriter(i.Output())
	i.SetOutput(switcher)
	if s, ok := switcher.Output().(net.SourceSetter); ok {
		s.SetSource(switcher)
	}

	return switcher, nil
}

func (mod *Module) findConnByNonce(nonce net.Nonce) *node.MonitoredConn {
	coreRouter, ok := mod.node.Router().(*node.CoreRouter)
	if !ok {
		return nil
	}

	for _, c := range coreRouter.Conns().All() {
		if c.Query().Nonce() == nonce {
			return c
		}
	}
	return nil
}

func (mod *Module) isLocal(identity id.Identity) bool {
	if mod.node.Identity().IsEqual(identity) {
		return true
	}
	for _, info := range mod.node.Services().List() {
		if info.Identity.IsEqual(identity) {
			return true
		}
	}
	return false
}

func (mod *Module) getRouter(w net.SecureWriteCloser) id.Identity {
	if final := net.FinalOutput(w); final != nil {
		return final.Identity()
	}
	return id.Identity{}
}