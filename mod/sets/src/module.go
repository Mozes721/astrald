package sets

import (
	"context"
	"errors"
	"github.com/cryptopunkscc/astrald/data"
	"github.com/cryptopunkscc/astrald/log"
	"github.com/cryptopunkscc/astrald/mod/sets"
	"github.com/cryptopunkscc/astrald/node"
	"github.com/cryptopunkscc/astrald/node/assets"
	"github.com/cryptopunkscc/astrald/node/events"
	"github.com/cryptopunkscc/astrald/sig"
	"github.com/cryptopunkscc/astrald/tasks"
	"gorm.io/gorm"
)

var _ sets.Module = &Module{}

type Module struct {
	config  Config
	node    node.Node
	log     *log.Logger
	assets  assets.Assets
	events  events.Queue
	db      *gorm.DB
	openers sig.Map[string, sets.Opener]

	universe  *UnionSet
	localnode *UnionSet
}

func (mod *Module) Run(ctx context.Context) error {
	return tasks.Group(
		&Service{Module: mod},
		tasks.RunFuncAdapter{RunFunc: mod.watchUnionMembers},
	).Run(ctx)
}

func (mod *Module) Edit(set string) (sets.Editor, error) {
	return NewEditor(mod, set)
}

func (mod *Module) Open(set string) (sets.Set, error) {
	var row dbSet
	var err = mod.db.Where("name = ?", set).First(&row).Error
	if err != nil {
		return nil, err
	}

	opener, ok := mod.openers.Get(row.Type)
	if !ok {
		return nil, errors.New("unsupported set type")
	}

	return opener(set)
}

func (mod *Module) openByID(id uint) (sets.Set, error) {
	var row dbSet
	var err = mod.db.Where("id = ?", id).First(&row).Error
	if err != nil {
		return nil, err
	}

	opener, ok := mod.openers.Get(row.Type)
	if !ok {
		return nil, errors.New("unsupported set type")
	}

	return opener(row.Name)
}

func (mod *Module) SetOpener(typ sets.Type, opener sets.Opener) {
	mod.openers.Replace(string(typ), opener)
}

func (mod *Module) GetOpener(typ sets.Type) sets.Opener {
	v, _ := mod.openers.Get(string(typ))
	return v
}

func (mod *Module) Create(name string, typ sets.Type) (sets.Set, error) {
	opener, found := mod.openers.Get(string(typ))
	if !found {
		return nil, errors.New("unsupported set type")
	}

	var row = dbSet{
		Name: name,
		Type: string(typ),
	}
	err := mod.db.Create(&row).Error
	if err != nil {
		return nil, err
	}

	set, err := opener(name)
	if err != nil {
		return nil, err
	}

	var info = &sets.Info{
		Name:      row.Name,
		Type:      sets.Type(row.Type),
		Size:      0,
		CreatedAt: row.CreatedAt,
	}

	mod.events.Emit(sets.EventSetCreated{Info: info})

	return set, nil
}

func (mod *Module) CreateBasic(name string, members ...data.ID) (sets.Basic, error) {
	s, err := mod.Create(name, sets.TypeBasic)
	if err != nil {
		return nil, err
	}
	set, ok := s.(*BasicSet)
	if !ok {
		panic("typecast failed")
	}

	return set, set.Add(members...)
}

func (mod *Module) CreateUnion(name string, members ...string) (sets.Union, error) {
	return mod.createUnion(name, members...)
}

func (mod *Module) Universe() sets.Union {
	return mod.universe
}

func (mod *Module) Localnode() sets.Union {
	return mod.localnode
}

func (mod *Module) createUnion(name string, members ...string) (*UnionSet, error) {
	s, err := mod.Create(name, sets.TypeUnion)
	if err != nil {
		return nil, err
	}
	set, ok := s.(*UnionSet)
	if !ok {
		panic("typecast failed")
	}

	return set, set.Add(members...)
}

func (mod *Module) SetInfo(name string) (*sets.Info, error) {
	setRow, err := mod.dbFindSetByName(name)
	if err != nil {
		return nil, err
	}

	var info = &sets.Info{
		Name:        setRow.Name,
		Type:        sets.Type(setRow.Type),
		Size:        -1,
		Visible:     setRow.Visible,
		Description: setRow.Description,
		CreatedAt:   setRow.CreatedAt,
	}

	var count int64

	var tx = mod.db.
		Model(&dbMember{}).
		Where("set_id = ? and removed = false", setRow.ID).
		Count(&count)

	if tx.Error != nil {
		return nil, tx.Error
	}

	info.Size = int(count)

	return info, nil
}

func (mod *Module) Where(dataID data.ID) ([]string, error) {
	dataRow, err := mod.dbDataFindOrCreateByDataID(dataID)
	if err != nil {
		return nil, err
	}

	var rows []dbMember
	err = mod.db.
		Where("data_id = ?", dataRow.ID).
		Find(&rows).Error

	var list []string

	for _, row := range rows {
		if row.Removed {
			list = append(list, row.Set.Name)
		}
	}

	return list, nil
}

func (mod *Module) All() ([]sets.Info, error) {
	var list []sets.Info

	var rows []dbSet
	var err = mod.db.Find(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		var count int64

		var tx = mod.db.
			Model(&dbMember{}).
			Where("set_id = ? and removed = false", row.ID).
			Count(&count)

		if tx.Error != nil {
			mod.log.Errorv(2, "error getting entry count: %v", tx.Error)
			count = -1
		}

		list = append(list, sets.Info{
			Name:        row.Name,
			Type:        sets.Type(row.Type),
			Size:        int(count),
			Visible:     row.Visible,
			Description: row.Description,
			CreatedAt:   row.CreatedAt,
		})
	}

	return list, err
}

func (mod *Module) Scan(name string, opts *sets.ScanOpts) ([]*sets.Member, error) {
	set, err := mod.Open(name)
	if err != nil {
		return nil, err
	}

	return set.Scan(opts)
}

func (mod *Module) SetVisible(name string, visible bool) error {
	return mod.db.
		Model(&dbSet{}).
		Where("name = ?", name).
		Update("visible", visible).
		Error
}

func (mod *Module) SetDescription(name string, desc string) error {
	return mod.db.
		Model(&dbSet{}).
		Where("name = ?", name).
		Update("description", desc).
		Error
}
