package admin

import (
	"bitbucket.org/creachadair/shell"
	"context"
	"errors"
	"github.com/cryptopunkscc/astrald/debug"
	"github.com/cryptopunkscc/astrald/log"
	"github.com/cryptopunkscc/astrald/net"
	"github.com/cryptopunkscc/astrald/node"
	"github.com/cryptopunkscc/astrald/node/assets"
	"github.com/cryptopunkscc/astrald/node/modules"
	"sync"
)

var _ modules.Module = &Module{}

const ServiceName = "admin"

type Module struct {
	config   Config
	node     node.Node
	assets   assets.Store
	commands map[string]Command
	log      *log.Logger
	mu       sync.Mutex
}

func (mod *Module) Run(ctx context.Context) error {
	service, err := mod.node.Services().Register(ctx, mod.node.Identity(), ServiceName, mod)
	if err != nil {
		return err
	}

	<-service.Done()

	return nil
}

func (mod *Module) RouteQuery(ctx context.Context, query net.Query, caller net.SecureWriteCloser) (net.SecureWriteCloser, error) {
	if query.Origin() != net.OriginLocal {
		return nil, net.ErrRejected
	}

	return net.Accept(query, caller, mod.serve)
}

func (mod *Module) serve(conn net.SecureConn) {
	defer debug.SaveLog(func(p any) {
		mod.log.Error("admin session panicked: %v", p)
	})

	defer conn.Close()

	var term = NewTerminal(conn, mod.log)

	for {
		term.Printf("%s@%s%s", conn.RemoteIdentity(), mod.node.Identity(), mod.config.Prompt)

		line, err := term.ScanLine()
		if err != nil {
			return
		}

		if err := mod.exec(line, term); err != nil {
			term.Printf("error: %v\n", err)
		} else {
			term.Printf("ok\n")
		}
	}
}

func (mod *Module) exec(line string, term *Terminal) error {
	args, valid := shell.Split(line)
	if len(args) == 0 {
		return nil
	}
	if !valid {
		return errors.New("unclosed quotes")
	}

	if cmd, found := mod.commands[args[0]]; found {
		return cmd.Exec(term, args)
	} else {
		return errors.New("command not found")
	}
}
