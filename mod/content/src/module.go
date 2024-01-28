package content

import (
	"context"
	"github.com/cryptopunkscc/astrald/data"
	"github.com/cryptopunkscc/astrald/log"
	"github.com/cryptopunkscc/astrald/mod/content"
	"github.com/cryptopunkscc/astrald/mod/fs"
	"github.com/cryptopunkscc/astrald/mod/index"
	"github.com/cryptopunkscc/astrald/mod/storage"
	"github.com/cryptopunkscc/astrald/node"
	"github.com/cryptopunkscc/astrald/node/events"
	"github.com/cryptopunkscc/astrald/sig"
	"gorm.io/gorm"
	"time"
)

var _ content.Module = &Module{}

const identifySize = 4096
const adcMethod = "adc"
const mimetypeMethod = "mimetype"

type Module struct {
	node   node.Node
	config Config
	log    *log.Logger
	events events.Queue
	db     *gorm.DB

	describers sig.Set[content.Describer]

	storage storage.Module
	fs      fs.Module
	index   index.Module

	ready chan struct{}
}

func (mod *Module) Run(ctx context.Context) error {
	<-ctx.Done()

	return nil
}

// Scan returns a channel that will be populated with all data entries since the provided timestamp and
// subscribed to any new items until context is done. If type is empty, all data entries will be passed regardless
// of the type.
func (mod *Module) Scan(ctx context.Context, opts *content.ScanOpts) <-chan *content.Info {
	if opts == nil {
		opts = &content.ScanOpts{}
	}

	if opts.After.After(time.Now()) {
		return nil
	}

	var ch = make(chan *content.Info)
	var subscription = mod.events.Subscribe(ctx)

	go func() {
		defer close(ch)

		// catch up with existing entries
		list, err := mod.find(opts)
		if err != nil {
			return
		}
		for _, item := range list {
			select {
			case ch <- item:
			case <-ctx.Done():
				return
			}
		}

		// subscribe to new items
		for event := range subscription {
			e, ok := event.(content.EventDataIdentified)
			if !ok {
				continue
			}
			if opts.Type != "" && e.Info.Type != opts.Type {
				continue
			}
			ch <- e.Info
		}
	}()

	return ch
}

func (mod *Module) Forget(dataID data.ID) error {
	var err = mod.db.Delete(&dbDataType{}, dataID).Error
	if err != nil {
		return err
	}

	return mod.index.RemoveFromSet(content.IdentifiedDataSetName, dataID)
}

func (mod *Module) Ready(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-mod.ready:
		return nil
	}
}

// find returns all data items indexed since time ts. If t is not empty, only items of type t will
// be returned.
func (mod *Module) find(opts *content.ScanOpts) ([]*content.Info, error) {
	var list []*content.Info
	var rows []*dbDataType

	if opts == nil {
		opts = &content.ScanOpts{}
	}

	var query = mod.db

	// filter by type if provided
	if opts.Type != "" {
		query = query.Where("type = ?", opts.Type)
	}

	// filter by time if provided
	if !opts.After.IsZero() {
		query = query.Where("indexed_at > ?", opts.After)
	}

	// fetch rows
	var tx = query.Order("indexed_at").Find(&rows)
	if tx.Error != nil {
		return nil, tx.Error
	}

	for _, row := range rows {
		list = append(list, &content.Info{
			DataID:    row.DataID,
			IndexedAt: row.IndexedAt,
			Method:    row.Method,
			Type:      row.Type,
		})
	}

	return list, nil
}

func (mod *Module) setReady() {
	close(mod.ready)
}
