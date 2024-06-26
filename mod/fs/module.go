package fs

import (
	"github.com/cryptopunkscc/astrald/object"
	"path/filepath"
	"time"
)

const (
	ModuleName = "fs"
	DBPrefix   = "fs__"
)

type Module interface {
	Find(opts *FindOpts) []*File
	Path(objectID object.ID) []string
}

type File struct {
	Path     string
	ObjectID object.ID
	ModTime  time.Time
}

type FindOpts struct {
	UpdatedAfter time.Time
}

type FileDesc struct {
	Paths []string
}

func (FileDesc) Type() string {
	return "mod.fs.file"
}
func (d FileDesc) String() string {
	if len(d.Paths) == 0 {
		return ""
	}
	return filepath.Base(d.Paths[0])
}
