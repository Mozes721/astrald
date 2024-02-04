package shares

import (
	"github.com/cryptopunkscc/astrald/mod/content"
	"github.com/cryptopunkscc/astrald/mod/keys"
	"github.com/cryptopunkscc/astrald/mod/media"
	"github.com/cryptopunkscc/astrald/mod/relay"
	"github.com/cryptopunkscc/astrald/mod/zip"
	"time"
)

type Config struct {
	NotifyDelay         time.Duration
	DescriptorWhitelist []string
}

var defaultConfig = Config{
	NotifyDelay: 10 * time.Second,
	DescriptorWhitelist: []string{
		content.TypeDescriptor{}.DescriptorType(),
		keys.KeyDescriptor{}.DescriptorType(),
		media.Descriptor{}.DescriptorType(),
		zip.ArchiveDescriptor{}.DescriptorType(),
		relay.CertDescriptor{}.DescriptorType(),
	},
}
