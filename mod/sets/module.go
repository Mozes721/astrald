package sets

import (
	"github.com/cryptopunkscc/astrald/object"
	"time"
)

const ModuleName = "sets"
const DBPrefix = "sets__"

type Module interface {
	Open(name string, create bool) (Set, error)
	Create(name string) (Set, error)

	All() ([]string, error)
	Where(object.ID) ([]string, error)
}

type Set interface {
	Name() string
	Scan(opts *ScanOpts) ([]*Member, error)
	Add(...object.ID) error
	Remove(...object.ID) error
	Delete() error
	Clear() error
	Trim(time.Time) error
	TrimmedAt() time.Time
	Stat() (*Stat, error)
}

type ScanOpts struct {
	UpdatedAfter   time.Time
	UpdatedBefore  time.Time
	IncludeRemoved bool
	ObjectID       object.ID
}

type Stat struct {
	Name      string
	Size      int
	DataSize  uint64
	CreatedAt time.Time
	TrimmedAt time.Time
}

type Member struct {
	ObjectID  object.ID
	Removed   bool
	UpdatedAt time.Time
}
