package auth

import (
	"github.com/cryptopunkscc/astrald/node/auth/brontide"
	"github.com/cryptopunkscc/astrald/node/auth/id"
	"github.com/cryptopunkscc/astrald/node/net"
)

type brontideConn struct {
	netConn        net.Conn
	bConn          *brontide.Conn
	remoteIdentity id.Identity
}

func (conn *brontideConn) Read(p []byte) (n int, err error) {
	return conn.bConn.Read(p)
}

func (conn *brontideConn) Write(p []byte) (n int, err error) {
	return conn.bConn.Write(p)
}

func (conn *brontideConn) Close() error {
	return conn.bConn.Close()
}

func (conn *brontideConn) Outbound() bool {
	return conn.netConn.Outbound()
}

func (conn *brontideConn) RemoteAddr() net.Addr {
	return conn.netConn.RemoteAddr()
}

func (conn *brontideConn) RemoteIdentity() id.Identity {
	return id.ECIdentityFromPublicKey(conn.bConn.RemotePub())
}