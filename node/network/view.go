package network

import (
	"github.com/cryptopunkscc/astrald/astral/link"
	"github.com/cryptopunkscc/astrald/auth/id"
	"github.com/cryptopunkscc/astrald/logfmt"
	"log"
	"sync"
)

type View struct {
	peers  map[string]*Peer
	peerMu sync.Mutex
	links  *link.Set
}

func NewView() *View {
	return &View{
		peers: make(map[string]*Peer),
		links: link.NewSet(),
	}
}

func (view *View) Peer(peerID id.Identity) *Peer {
	view.peerMu.Lock()
	defer view.peerMu.Unlock()

	hex := peerID.PublicKeyHex()
	if peer, found := view.peers[hex]; found {
		return peer
	}
	view.peers[hex] = NewPeer(peerID)
	return view.peers[hex]
}

func (view *View) AddLink(link *link.Link) error {
	if err := view.links.Add(link); err == nil {
		log.Println("link up", logfmt.ID(link.RemoteIdentity()), logfmt.Dir(link.Outbound()), "via", link.RemoteAddr().Network(), link.RemoteAddr().String())

		go func() {
			<-link.WaitClose()
			log.Println("link down", logfmt.ID(link.RemoteIdentity()), logfmt.Dir(link.Outbound()), "via", link.RemoteAddr().Network(), link.RemoteAddr().String())
			view.links.Remove(link)
		}()
	}
	_ = view.Peer(link.RemoteIdentity()).AddLink(link)

	return nil
}

func (view *View) Links() <-chan *link.Link {
	return view.links.Each()
}

func (view *View) Peers() <-chan *Peer {
	view.peerMu.Lock()
	defer view.peerMu.Unlock()

	ch := make(chan *Peer, len(view.peers))
	for _, p := range view.peers {
		ch <- p
	}
	close(ch)
	return ch
}