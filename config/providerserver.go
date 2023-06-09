package config

type ProviderServer struct {
	// ListenMultiaddr is the multiaddr string for the node's listen address
	ListenMultiaddr string
	// ControlAddr is where to run the http control server
	ControlAddr string
	// RetrievalMultiaddrs are the addresses to advertise for data retrieval.
	// Defaults to the provider's libp2p host listen addresses.
	RetrievalMultiaddrs []string
}

// NewProviderServer instantiates a new ProviderServer config with default values.
func NewProviderServer() ProviderServer {
	return ProviderServer{
		ListenMultiaddr: "/ip4/0.0.0.0/tcp/3103",
		ControlAddr:     ":3104",
	}
}

// PopulateDefaults replaces zero-values in the config with default values.
func (c *ProviderServer) PopulateDefaults() {
	def := NewProviderServer()

	if c.ListenMultiaddr == "" {
		c.ListenMultiaddr = def.ListenMultiaddr
	}
	if c.ControlAddr == "" {
		c.ControlAddr = def.ControlAddr
	}
}
