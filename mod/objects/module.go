package objects

import (
	"context"
	"github.com/cryptopunkscc/astrald/auth/id"
	"github.com/cryptopunkscc/astrald/lib/desc"
	"github.com/cryptopunkscc/astrald/net"
	"github.com/cryptopunkscc/astrald/object"
)

const ModuleName = "objects"
const DBPrefix = "objects__"

// ReadAllMaxSize is the size limit for loading objects into memory
const ReadAllMaxSize = 64 * 1024 * 1024 // 64 MB

type Module interface {
	// AddOpener registers an Opener. Openers are queried from highest to lowest priority.
	AddOpener(opener Opener, priority int) error
	Opener

	// AddCreator registers a Creator. Creators are queried from highest to lowest priority.
	AddCreator(creator Creator, priority int) error
	Creator

	AddDescriber(Describer) error
	Describer

	AddPurger(purger Purger) error
	Purger

	AddSearcher(Searcher) error
	Searcher

	AddFinder(Finder) error
	Finder

	Encode(obj Object) ([]byte, error)
	Decode(data []byte) (Object, error)
	SetDecoder(string, Decoder) error

	// Load decodes the object into memory and returns it
	Load(context.Context, object.ID, *net.Scope) (Object, error)

	// Store encodes the object to local storage
	Store(context.Context, Object) (object.ID, error)

	AddPrototypes(protos ...desc.Data) error
	UnmarshalDescriptor(name string, buf []byte) desc.Data

	// Get reads the whole object into memory and returns the buffer
	Get(id object.ID, opts *OpenOpts) ([]byte, error)

	// Put commits the object to storage and returns its ID
	Put(object []byte, opts *CreateOpts) (object.ID, error)

	// Hold marks the identity as a holder of the objects
	Hold(id.Identity, ...object.ID) error

	// Release clears the identity as a holder of the objects
	Release(id.Identity, ...object.ID) error

	// Holders returns a list of holders of an object
	Holders(object.ID) []id.Identity

	// Holdings returns a list of objects held by an identity
	Holdings(id.Identity) []object.ID

	Connect(caller id.Identity, target id.Identity) (Consumer, error)
}

type Consumer interface {
	Describe(context.Context, object.ID, *desc.Opts) ([]*desc.Desc, error)
	Open(context.Context, object.ID, *OpenOpts) (Reader, error)
	Put(context.Context, []byte) (object.ID, error)
	Search(context.Context, string) ([]Match, error)
}

type Describer interface {
	Describe(ctx context.Context, object object.ID, opts *desc.Opts) []*desc.Desc
}

type Purger interface {
	Purge(object.ID, *PurgeOpts) (int, error)
}

type PurgeOpts struct {
	// for future use
}
