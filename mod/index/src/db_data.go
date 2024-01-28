package index

import (
	"github.com/cryptopunkscc/astrald/data"
	"time"
)

type dbData struct {
	ID        uint    `gorm:"primarykey"`
	DataID    data.ID `gorm:"uniqueIndex"`
	CreatedAt time.Time
}

func (dbData) TableName() string { return "data" }

func (mod *Module) dbDataFindByDataID(dataID data.ID) (*dbData, error) {
	var row dbData
	var tx = mod.db.Where("data_id = ?", dataID).First(&row)
	return &row, tx.Error
}

func (mod *Module) dbDataCreate(dataID data.ID) (*dbData, error) {
	var row = dbData{DataID: dataID}
	var tx = mod.db.Create(&row)
	return &row, tx.Error
}

func (mod *Module) dbDataFindOrCreateByDataID(dataID data.ID) (*dbData, error) {
	if row, err := mod.dbDataFindByDataID(dataID); err == nil {
		return row, nil
	}

	return mod.dbDataCreate(dataID)
}
