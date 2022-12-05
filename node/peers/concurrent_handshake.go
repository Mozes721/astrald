package peers

import (
	"context"
	"errors"
	"github.com/cryptopunkscc/astrald/auth"
	"github.com/cryptopunkscc/astrald/auth/id"
	ainfra "github.com/cryptopunkscc/astrald/infra"
	"io"
	"log"
	"sync"
)

type ConcurrentHandshake struct {
	localID  id.Identity
	remoteID id.Identity
	workers  int
}

func NewConcurrentHandshake(localID id.Identity, remoteID id.Identity, workers int) *ConcurrentHandshake {
	return &ConcurrentHandshake{localID: localID, remoteID: remoteID, workers: workers}
}

func (h *ConcurrentHandshake) Outbound(ctx context.Context, conns <-chan ainfra.Conn) <-chan auth.Conn {
	var ch = make(chan auth.Conn)
	var wg sync.WaitGroup

	// start handshake workers
	wg.Add(h.workers)
	for i := 0; i < h.workers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case conn, ok := <-conns:
					if !ok {
						return
					}

					authConn, err := auth.HandshakeOutbound(ctx, conn, h.remoteID, h.localID)

					// if handshake failed, try next connection
					if err != nil {
						if !errors.Is(err, io.EOF) {
							log.Println("peers.ConcurrentHandshake.Outbound(): handshake error:", err)
						}
						conn.Close()
						continue
					}

					ch <- authConn
					return
				}

			}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
		for conn := range conns {
			conn.Close()
		}
	}()

	return ch
}