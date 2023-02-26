package node

import (
	"fmt"
	"github.com/cryptopunkscc/astrald/auth/id"
	"github.com/cryptopunkscc/astrald/hub"
	_log "github.com/cryptopunkscc/astrald/log"
	"github.com/cryptopunkscc/astrald/node/config"
	"github.com/cryptopunkscc/astrald/node/contacts"
	"github.com/cryptopunkscc/astrald/node/db"
	"github.com/cryptopunkscc/astrald/node/event"
	"github.com/cryptopunkscc/astrald/node/infra"
	"github.com/cryptopunkscc/astrald/node/peers"
	"github.com/cryptopunkscc/astrald/node/presence"
	"github.com/cryptopunkscc/astrald/node/tracker"
	"os"
	"path/filepath"
	"time"
)

const defaultQueryTimeout = time.Minute
const dbFileName = "astrald.db"
const configFileName = "astrald.conf"

type Node struct {
	events   event.Queue
	Config   config.Config
	Database *db.Database
	identity id.Identity

	Infra    *infra.Infra
	Tracker  *tracker.Tracker
	Contacts *contacts.Manager
	Ports    *hub.Hub
	Modules  *ModuleManager
	Peers    *peers.Manager
	Presence *presence.Manager

	rootDir string
}

func (node *Node) Events() *event.Queue {
	return &node.events
}

var log = _log.Tag("node")

func New(rootDir string, modules ...ModuleLoader) (*Node, error) {
	var err error
	var node = &Node{
		rootDir: rootDir,
	}

	// load config
	filePath := filepath.Join(rootDir, configFileName)
	node.Config, err = config.LoadYAMLFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}

	for tag, level := range node.Config.Log.TagLevels {
		_log.SetTagLevel(tag, level)
	}
	for tag, color := range node.Config.Log.TagColors {
		_log.SetTagColor(tag, color)
	}
	_log.HideDate = node.Config.Log.HideDate
	_log.Level = node.Config.Log.Level

	// setup database
	var dbInit bool
	dbFile := filepath.Join(rootDir, dbFileName)
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		log.Log("creating database at %s", dbFile)
		dbInit = true
	}

	node.Database, err = db.NewFileDatabase(dbFile)
	if err != nil {
		return nil, fmt.Errorf("db error: %w", err)
	}

	if dbInit {
		if err := tracker.InitDatabase(node.Database); err != nil {
			return nil, fmt.Errorf("tracker: %w", err)
		}
		if err := contacts.InitDatabase(node.Database); err != nil {
			return nil, fmt.Errorf("contacts: %w", err)
		}

		if err := os.Chmod(dbFile, 0600); err != nil {
			log.Error("cannot set 0600 mode on the database file: %s", err)
		}
	}

	// identity
	if err := node.setupIdentity(); err != nil {
		return nil, fmt.Errorf("error setting up identity: %w", err)
	}

	// hub
	node.Ports = hub.New(&node.events)

	// infrastructure
	node.Infra, err = infra.New(
		node.Identity(),
		node.Config.Infra,
		infra.FilteredQuerier{Querier: node, FilteredID: node.identity},
		node.RootDir(),
	)
	if err != nil {
		return nil, fmt.Errorf("error setting up infrastructure: %w", err)
	}

	// tracker
	node.Tracker, err = tracker.New(node.Database, node.Infra)
	if err != nil {
		return nil, err
	}

	// contacts
	node.Contacts, err = contacts.New(node.Database, &node.events)
	if err != nil {
		return nil, err
	}

	_log.SetFormatter(id.Identity{}, func(v interface{}) string {
		identity := v.(id.Identity)
		if c, err := node.Contacts.Find(identity); err == nil {
			if c.Alias() != "" {
				return log.Cyan() + c.Alias() + log.Reset()
			}
		}

		return log.Green() + identity.Fingerprint() + log.Reset()
	})

	// peer manager
	node.Peers, err = peers.NewManager(node.identity, node.Infra, node.Tracker, &node.events)
	if err != nil {
		return nil, fmt.Errorf("error setting up peer manager: %w", err)
	}

	// presence
	node.Presence, err = presence.NewManager(node.Infra, &node.events)
	if err != nil {
		return nil, fmt.Errorf("error setting up presence: %w", err)
	}

	// modules
	node.Modules, err = NewModuleManager(node, modules)
	if err != nil {
		return nil, fmt.Errorf("error creating module manager: %w", err)
	}

	return node, nil
}

func (node *Node) RootDir() string {
	return node.rootDir
}

func (node *Node) Identity() id.Identity {
	return node.identity
}

func (node *Node) Alias() string {
	return node.Config.GetAlias()
}

func (node *Node) SetAlias(alias string) error {
	//TODO: unimplemented
	return nil
}
