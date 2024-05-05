package objects

import (
	"errors"
	"github.com/cryptopunkscc/astrald/lib/arl"
	"github.com/cryptopunkscc/astrald/mod/objects"
	"github.com/cryptopunkscc/astrald/net"
	"github.com/cryptopunkscc/astrald/object"
	"io"
	"net/http"
)

func (mod *Module) Fetch(addr string) (objectID object.ID, err error) {
	switch {
	case isURL(addr):
		return mod.FetchURL(addr)

	case isARL(addr):
		var a *arl.ARL

		a, err = arl.Parse(addr, mod.node.Resolver())
		if err != nil {
			return
		}
		return mod.FetchARL(a)
	}

	return objectID, errors.New("scheme not supported")
}

func (mod *Module) FetchURL(url string) (objectID object.ID, err error) {
	// Make a GET request to the URL
	response, err := http.Get(url)
	if err != nil {
		return
	}
	defer response.Body.Close()

	var alloc = max(response.ContentLength, 0)

	w, err := mod.Create(
		&objects.CreateOpts{
			Alloc: int(alloc),
		},
	)
	if err != nil {
		return
	}
	defer w.Discard()

	_, err = io.Copy(w, response.Body)
	if err != nil {
		return
	}

	return w.Commit()
}

func (mod *Module) FetchARL(a *arl.ARL) (objectID object.ID, err error) {
	if a.Caller.IsZero() {
		a.Caller = mod.node.Identity()
	}

	var query = net.NewQuery(a.Caller, a.Target, a.Query)

	conn, err := net.Route(mod.ctx, mod.node.Router(), query)
	if err != nil {
		return
	}

	w, err := mod.Create(nil)
	if err != nil {
		return
	}
	defer w.Discard()

	io.Copy(w, conn)

	return w.Commit()
}