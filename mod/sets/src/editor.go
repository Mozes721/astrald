package sets

import (
	"errors"
	"fmt"
	"github.com/cryptopunkscc/astrald/data"
	"github.com/cryptopunkscc/astrald/mod/sets"
	"github.com/cryptopunkscc/astrald/sig"
	"time"
)

var _ sets.Editor = &Editor{}

type Editor struct {
	*Module
	set *dbSet
}

func NewEditor(mod *Module, set string) (*Editor, error) {
	var row dbSet
	var err = mod.db.Where("name = ?", set).First(&row).Error
	if err != nil {
		return nil, err
	}

	return &Editor{Module: mod, set: &row}, nil
}

func (e *Editor) Add(dataIDs ...data.ID) error {
	var ids []uint

	for _, dataID := range dataIDs {
		row, err := e.dbDataFindOrCreateByDataID(dataID)
		if err != nil {
			return err
		}
		ids = append(ids, row.ID)
	}
	return e.AddByID(ids...)
}

func (e *Editor) Remove(dataIDs ...data.ID) error {
	var ids []uint

	for _, dataID := range dataIDs {
		row, err := e.dbDataFindOrCreateByDataID(dataID)
		if err != nil {
			return err
		}
		ids = append(ids, row.ID)
	}
	return e.RemoveByID(ids...)
}

func (e *Editor) Scan(opts *sets.ScanOpts) ([]*sets.Member, error) {
	if opts == nil {
		opts = &sets.ScanOpts{}
	}

	var rows []dbMember
	var q = e.db.
		Where("set_id = ?", e.set.ID).
		Order("updated_at").
		Preload("Data")

	if !opts.UpdatedAfter.IsZero() {
		q = q.Where("updated_at > ?", opts.UpdatedAfter)
	}
	if !opts.UpdatedBefore.IsZero() {
		q = q.Where("updated_at < ?", opts.UpdatedBefore)
	}
	if !opts.IncludeRemoved {
		q = q.Where("removed = false")
	}
	if !opts.DataID.IsZero() {
		row, err := e.dbDataFindByDataID(opts.DataID)
		if err != nil {
			return nil, err
		}
		q = q.Where("data_id = ?", row.ID)
	}

	err := q.Find(&rows).Error
	if err != nil {
		return nil, err
	}

	var entries []*sets.Member

	for _, row := range rows {
		entries = append(entries, &sets.Member{
			DataID:    row.Data.DataID,
			Removed:   row.Removed,
			UpdatedAt: row.UpdatedAt,
		})
	}

	return entries, nil
}

func (e *Editor) Trim(t time.Time) error {
	if t.After(time.Now()) {
		return errors.New("invalid time")
	}

	if !t.After(e.set.TrimmedAt) {
		return errors.New("already trimmed with later date")
	}

	e.set.TrimmedAt = t
	err := e.db.Save(e.set).Error
	if err != nil {
		return err
	}

	err = e.db.
		Where("removed = true AND updated_at < ?", t).
		Delete(&dbMember{}).Error

	return err
}

func (e *Editor) TrimmedAt() time.Time {
	return e.set.TrimmedAt
}

func (e *Editor) Reset() error {
	var err error
	var ids []uint

	err = e.db.
		Model(&dbMember{}).
		Select("data_id").
		Where("set_id = ?", e.set.ID).
		Find(&ids).Error
	if err != nil {
		return err
	}

	err = e.RemoveByID(ids...)
	if err != nil {
		return err
	}

	e.set.TrimmedAt = time.Now()
	err = e.db.Save(e.set).Error

	e.events.Emit(eventSetUpdated{row: e.set})
	e.events.Emit(sets.EventSetUpdated{Name: e.set.Name})

	return err
}

func (e *Editor) AddByID(ids ...uint) error {
	var err error
	var duplicates []uint
	var removedIDs []uint

	// filter out elements that are already added
	err = e.db.
		Model(&dbMember{}).
		Select("data_id").
		Where("removed = false AND set_id = ? AND data_id IN (?)", e.set.ID, ids).
		Find(&duplicates).Error
	if err != nil {
		return err
	}

	ids = subtract(ids, duplicates)

	// fetch existing rows marked as removed
	err = e.db.
		Model(&dbMember{}).
		Select("data_id").
		Where("removed = true AND set_id = ? AND data_id IN (?)", e.set.ID, ids).
		Find(&removedIDs).Error
	if err != nil {
		return err
	}

	var insertIDs = subtract(ids, removedIDs)

	err = e.unremove(removedIDs)
	if err != nil {
		return err
	}

	err = e.insert(insertIDs)
	if err != nil {
		return err
	}

	e.events.Emit(eventSetUpdated{row: e.set})
	e.events.Emit(sets.EventSetUpdated{Name: e.set.Name})

	return nil
}

func (e *Editor) RemoveByID(ids ...uint) error {
	if len(ids) == 0 {
		return nil
	}

	var err error
	var clean []uint

	// reduce the id set to members actually in the set
	err = e.db.
		Model(&dbMember{}).
		Select("data_id").
		Where("removed = false AND set_id = ? AND data_id IN (?)", e.set.ID, ids).
		Find(&clean).
		Error

	// update members as removed from the set
	err = e.db.
		Model(&dbMember{}).
		Where("removed = false AND set_id = ? AND data_id IN (?)", e.set.ID, clean).
		Update("removed", true).Error
	if err != nil {
		return fmt.Errorf("update error: %w", err)
	}

	// fetch details about the removed rows
	var rows []dbMember
	err = e.db.
		Preload("Data").
		Where("set_id = ? AND data_id IN (?)", e.set.ID, clean).
		Find(&rows).Error
	if err != nil {
		return err
	}

	// emit an event for every removed member
	for _, row := range rows {
		e.events.Emit(sets.EventMemberUpdate{
			Set:       e.set.Name,
			DataID:    row.Data.DataID,
			Removed:   row.Removed,
			UpdatedAt: row.UpdatedAt,
		})
	}

	e.events.Emit(eventSetUpdated{row: e.set})
	e.events.Emit(sets.EventSetUpdated{Name: e.set.Name})

	return nil
}

func (e *Editor) Delete() error {
	var err error
	var rows []dbMember

	var lastState dbSet
	err = e.db.
		Preload("InclusionsAsSuper").
		Preload("InclusionsAsSub").
		Where("name = ?", e.set.Name).
		First(&lastState).
		Error

	// fetch members
	err = e.db.
		Preload("Data").
		Where("removed = false AND set_id = ?", e.set.ID).
		Find(&rows).Error
	if err != nil {
		return err
	}

	err = e.db.
		Where("set_id = ?", e.set.ID).
		Delete(&dbMember{}).Error
	if err != nil {
		return err
	}

	// emit an event for every removed member
	for _, row := range rows {
		e.events.Emit(sets.EventMemberUpdate{
			Set:       e.set.Name,
			DataID:    row.Data.DataID,
			Removed:   true,
			UpdatedAt: row.UpdatedAt,
		})
	}

	// delete inclusions
	err = e.db.
		Where("subset_id = ? or superset_id = ?", e.set.ID, e.set.ID).
		Delete(&dbSetInclusion{}).Error
	if err != nil {
		return err
	}

	err = e.db.Delete(e.set).Error
	if err != nil {
		return err
	}

	e.events.Emit(eventSetDeleted{row: &lastState})
	e.events.Emit(sets.EventSetDeleted{Name: e.set.Name})

	return nil
}

func (e *Editor) unremove(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}

	var err error

	// update rows
	err = e.db.
		Model(&dbMember{}).
		Where("removed = true AND set_id = ? AND data_id IN (?)", e.set.ID, ids).
		Update("removed", false).Error
	if err != nil {
		return fmt.Errorf("update error: %w", err)
	}

	var rows []dbMember

	// reload rows to include DataID
	err = e.db.
		Preload("Data").
		Where("set_id = ? AND data_id IN (?)", e.set.ID, ids).
		Find(&rows).Error
	if err != nil {
		return err
	}

	// emit an event for every updated member
	for _, row := range rows {
		e.events.Emit(sets.EventMemberUpdate{
			Set:       e.set.Name,
			DataID:    row.Data.DataID,
			Removed:   row.Removed,
			UpdatedAt: row.UpdatedAt,
		})
	}

	return nil
}

func (e *Editor) insert(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}

	var err error

	// create a slice of rows
	rows, _ := sig.MapSlice(ids, func(id uint) (*dbMember, error) {
		return &dbMember{
			DataID: id,
			SetID:  e.set.ID,
		}, nil
	})

	err = e.db.Create(&rows).Error
	if err != nil {
		return err
	}

	// reload rows to include Data
	err = e.db.
		Preload("Data").
		Where("set_id = ? AND data_id IN (?)", e.set.ID, ids).
		Find(&rows).Error
	if err != nil {
		return err
	}

	// emit an event for every updated  member
	for _, row := range rows {
		e.events.Emit(sets.EventMemberUpdate{
			Set:       e.set.Name,
			DataID:    row.Data.DataID,
			Removed:   row.Removed,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return nil
}

func subtract(set, subset []uint) []uint {
	var res []uint
	var i, j int

	for i < len(set) && j < len(subset) {
		if set[i] < subset[j] {
			res = append(res, set[i])
			i++
		} else if set[i] > subset[j] {
			j++
		} else {
			// If elements are equal, move to the next element in arr1
			i++
			j++
		}
	}

	// Append the remaining elements from set
	res = append(res, set[i:]...)

	return res
}
