package shares

import (
	"context"
	"errors"
	"fmt"
	"github.com/cryptopunkscc/astrald/auth/id"
	"github.com/cryptopunkscc/astrald/cslq"
	"github.com/cryptopunkscc/astrald/data"
	"github.com/cryptopunkscc/astrald/mod/sets"
	"github.com/cryptopunkscc/astrald/mod/shares"
	"github.com/cryptopunkscc/astrald/net"
	"strconv"
	"strings"
	"time"
)

var _ sets.Set = &RemoteShare{}
var _ shares.RemoteShare = &RemoteShare{}

type RemoteShare struct {
	mod    *Module
	caller id.Identity
	target id.Identity
	set    sets.Editor
	row    *dbRemoteShare
}

func (mod *Module) setOpener(name string) (sets.Set, error) {
	after, ok := strings.CutPrefix(name, remoteShareSetPrefix+".")
	if !ok {
		return nil, errors.New("invalid set name")
	}

	idstr := strings.Split(after, ".")
	if len(idstr) != 2 {
		return nil, errors.New("invalid set name")
	}

	caller, err := id.ParsePublicKeyHex(idstr[0])
	if err != nil {
		return nil, err
	}

	target, err := id.ParsePublicKeyHex(idstr[1])
	if err != nil {
		return nil, err
	}

	var share = &RemoteShare{
		mod:    mod,
		caller: caller,
		target: target,
	}

	share.set, err = mod.sets.Edit(name)
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (mod *Module) CreateRemoteShare(caller id.Identity, target id.Identity) (*RemoteShare, error) {
	var share = &RemoteShare{mod: mod, caller: caller, target: target}

	var row = dbRemoteShare{
		Caller: caller,
		Target: target,
	}

	var err = mod.db.Create(&row).Error
	if err != nil {
		return nil, err
	}
	share.row = &row

	setName := share.setName()

	_, err = mod.sets.Create(setName, shares.SetType)
	if err != nil {
		return nil, err
	}

	share.set, err = mod.sets.Edit(setName)
	if err != nil {
		return nil, err
	}

	mod.remoteShares.Add(setName)
	mod.sets.SetVisible(setName, true)
	mod.sets.SetDescription(setName,
		fmt.Sprintf(
			"Data shared with %s by %s",
			mod.node.Resolver().DisplayName(caller),
			mod.node.Resolver().DisplayName(target),
		),
	)

	return share, nil
}

func (mod *Module) FindRemoteShare(caller id.Identity, target id.Identity) (shares.RemoteShare, error) {
	return mod.findRemoteShare(caller, target)
}

func (mod *Module) findRemoteShare(caller id.Identity, target id.Identity) (*RemoteShare, error) {
	var row dbRemoteShare
	var err = mod.db.
		Where("caller = ? AND target = ?", caller, target).
		First(&row).Error
	if err != nil {
		return nil, err
	}

	var share = &RemoteShare{mod: mod, caller: caller, target: target, row: &row}
	share.set, err = mod.sets.Edit(share.setName())
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (mod *Module) FindOrCreateRemoteShare(caller id.Identity, target id.Identity) (*RemoteShare, error) {
	if share, err := mod.findRemoteShare(caller, target); err == nil {
		return share, nil
	}
	return mod.CreateRemoteShare(caller, target)
}

func (share *RemoteShare) Sync() error {
	if share.target.IsEqual(share.mod.node.Identity()) {
		return errors.New("cannot sync with self")
	}

	remoteShare, err := share.mod.FindOrCreateRemoteShare(share.caller, share.target)
	if err != nil {
		return err
	}

	var timestamp = "0"
	if !remoteShare.row.LastUpdate.IsZero() {
		timestamp = strconv.FormatInt(remoteShare.row.LastUpdate.UnixNano(), 10)
	}

	var query = net.NewQuery(share.caller, share.target, syncServicePrefix+timestamp)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := net.Route(ctx, share.mod.node.Router(), query)
	if err != nil {
		return err
	}
	defer conn.Close()

	for {
		var op byte
		err = cslq.Decode(conn, "c", &op)
		if err != nil {
			return err
		}

		switch op {
		case 0: // done
			var timestamp int64
			err = cslq.Decode(conn, "q", &timestamp)
			if err != nil {
				return err
			}

			return remoteShare.SetLastUpdate(time.Unix(0, timestamp))

		case 1: // add
			var dataID data.ID
			err = cslq.Decode(conn, "v", &dataID)
			if err != nil {
				return err
			}

			var tx = share.mod.db.Create(&dbRemoteData{
				Caller: share.caller,
				Target: share.target,
				DataID: dataID,
			})
			if tx.Error != nil {
				share.mod.log.Errorv(1, "sync: error adding remote data: %v", tx.Error)
			}

			remoteShare.set.Add(dataID)

		case 2: // remove
			var dataID data.ID
			err = cslq.Decode(conn, "v", &dataID)
			if err != nil {
				return err
			}

			var tx = share.mod.db.Delete(&dbRemoteData{
				Caller: share.caller,
				Target: share.target,
				DataID: dataID,
			})

			if tx.Error != nil {
				share.mod.log.Errorv(1, "sync: error removing remote data: %v", tx.Error)
			}

			remoteShare.set.Remove(dataID)

		default:
			return errors.New("protocol error")
		}
	}

}

func (share *RemoteShare) Unsync() error {
	var err error

	err = share.mod.db.
		Model(&dbRemoteData{}).
		Delete("caller = ? and target = ?", share.caller, share.target).
		Error
	if err != nil {
		return err
	}

	err = share.mod.db.Delete(&share.row).Error
	if err != nil {
		return err
	}

	return share.set.Delete()
}

func (share *RemoteShare) Scan(opts *sets.ScanOpts) ([]*sets.Member, error) {
	return share.set.Scan(opts)
}

func (share *RemoteShare) LastUpdate() time.Time {
	return share.row.LastUpdate
}

func (share *RemoteShare) SetLastUpdate(t time.Time) error {
	share.row.LastUpdate = t
	return share.mod.db.Save(&share.row).Error
}

func (share *RemoteShare) Info() (*sets.Info, error) {
	return &sets.Info{
		Name:        share.setName(),
		Type:        "share",
		Size:        0,
		Visible:     false,
		Description: "",
		CreatedAt:   time.Time{},
	}, nil
}

func (share *RemoteShare) setName() string {
	return fmt.Sprintf("%v.%v.%v",
		remoteShareSetPrefix,
		share.caller.PublicKeyHex(),
		share.target.PublicKeyHex(),
	)
}
