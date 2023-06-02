package config

import (
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

const (
	// Keep 1024 chunks in cache; keeps 256MiB if chunks are 0.25MiB.
	defaultLinkCacheSize = 1024
	// Multihashes are 128 bytes so 16384 results in 0.25MiB chunk when full.
	defaultLinkedChunkSize = 16384
	defaultPubSubTopic     = "/indexer/ingest/mainnet"
)

// Ingest configures settings related to the ingestion protocol.
type Ingest struct {
	// LinkCacheSize is the maximum number of links that cash can store before
	// LRU eviction.  If a single linked list has more links than the cache can
	// hold, the cache is resized to be able to hold all links.
	LinkCacheSize int
	// LinkedChunkSize is the number of multihashes in each chunk of in the
	// advertised entries linked list.  If multihashes are 128 bytes, then
	// setting LinkedChunkSize = 16384 will result in blocks of about 2Mb when
	// full.
	LinkedChunkSize int
	// PurgeLinkCache tells whether to purge the link cache on daemon startup.
	PurgeLinkCache bool

	// AnnounceMultiaddr is the address supplied in the announce message
	// telling indexers the address to use to retrieve advertisements. If not
	// specified, the ListenMultiaddr is used.
	AnnounceMultiaddr string
	// ListenMultiaddr is the address of the interface to listen for HTTP
	// requests for advertisements.
	ListenMultiaddr string
}

// NewIngest instantiates a new Ingest configuration with default values.
func NewIngest() Ingest {
	return Ingest{
		LinkCacheSize:   defaultLinkCacheSize,
		LinkedChunkSize: defaultLinkedChunkSize,
		ListenMultiaddr: "/ip4/0.0.0.0/tcp/3104/http",
	}
}

// PopulateDefaults replaces zero-values in the config with default values.
func (c *Ingest) PopulateDefaults() {
	if c.LinkCacheSize == 0 {
		c.LinkCacheSize = defaultLinkCacheSize
	}
	if c.LinkedChunkSize == 0 {
		c.LinkedChunkSize = defaultLinkedChunkSize
	}
}

func (c *Ingest) ListenNetAddr() (string, error) {
	maddr, err := multiaddr.NewMultiaddr(c.ListenMultiaddr)
	if err != nil {
		return "", err
	}
	httpMultiaddr, _ := multiaddr.NewMultiaddr("/http")
	maddr = maddr.Decapsulate(httpMultiaddr)

	netAddr, err := manet.ToNetAddr(maddr)
	if err != nil {
		return "", err
	}
	return netAddr.String(), nil
}
