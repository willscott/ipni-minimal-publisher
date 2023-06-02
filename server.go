package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	datatransfer "github.com/filecoin-project/go-data-transfer/v2/impl"
	dtnetwork "github.com/filecoin-project/go-data-transfer/v2/network"
	gstransport "github.com/filecoin-project/go-data-transfer/v2/transport/graphsync"
	leveldb "github.com/ipfs/go-ds-leveldb"
	gsimpl "github.com/ipfs/go-graphsync/impl"
	gsnet "github.com/ipfs/go-graphsync/network"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipfs/kubo/core/bootstrap"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/index-provider/engine"
	"github.com/libp2p/go-libp2p"
	"github.com/mitchellh/go-homedir"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"

	"github.com/willscott/ipni-minimal-publisher/config"
)

var log = logging.Logger("command/reference-provider")

var (
	ErrDaemonStart  = errors.New("daemon did not start correctly")
	ErrDaemonStop   = errors.New("daemon did not stop gracefully")
	ErrCleanupFiles = errors.New("unable to cleanup temporary files correctly")
)

const (
	// shutdownTimeout is the duration that a graceful shutdown has to complete
	shutdownTimeout = 5 * time.Second
)

var serverFlags = []cli.Flag{
	&cli.StringFlag{
		Name:     "log-level",
		Usage:    "Set the log level",
		EnvVars:  []string{"GOLOG_LOG_LEVEL"},
		Value:    "info",
		Required: false,
	},
}

func serverCommand(cctx *cli.Context) error {
	err := logging.SetLogLevel("*", cctx.String("log-level"))
	if err != nil {
		return err
	}

	cfg, err := config.Load("")
	if err != nil {
		if errors.Is(err, config.ErrNotInitialized) {
			return errors.New("reference provider is not initialized\nTo initialize, run using the \"init\" command")
		}
		return fmt.Errorf("cannot load config file: %w", err)
	}

	// Initialize libp2p host
	ctx, cancelp2p := context.WithCancel(cctx.Context)
	defer cancelp2p()

	peerID, privKey, err := cfg.Identity.DecodeOrCreate(cctx.App.Writer)
	if err != nil {
		return err
	}

	p2pmaddr, err := multiaddr.NewMultiaddr(cfg.ProviderServer.ListenMultiaddr)
	if err != nil {
		return fmt.Errorf("bad p2p address in config %s: %s", cfg.ProviderServer.ListenMultiaddr, err)
	}
	h, err := libp2p.New(
		// Use the keypair generated during init
		libp2p.Identity(privKey),
		// Listen to p2p addr specified in config
		libp2p.ListenAddrs(p2pmaddr),
	)
	if err != nil {
		return err
	}
	log.Infow("libp2p host initialized", "host_id", h.ID(), "multiaddr", p2pmaddr)

	// Initialize datastore
	if cfg.Datastore.Type != "levelds" {
		return fmt.Errorf("only levelds datastore type supported, %q not supported", cfg.Datastore.Type)
	}
	dataStorePath, err := config.Path("", cfg.Datastore.Dir)
	if err != nil {
		return err
	}
	err = dirWritable(dataStorePath)
	if err != nil {
		return err
	}
	ds, err := leveldb.NewDatastore(dataStorePath, nil)
	if err != nil {
		return err
	}

	gsnet := gsnet.NewFromLibp2pHost(h)
	dtNet := dtnetwork.NewFromLibp2pHost(h)
	gs := gsimpl.New(context.Background(), gsnet, cidlink.DefaultLinkSystem())
	tp := gstransport.NewTransport(h.ID(), gs)
	dt, err := datatransfer.NewDataTransfer(ds, dtNet, tp)
	if err != nil {
		return err
	}
	err = dt.Start(context.Background())
	if err != nil {
		return err
	}

	httpListenAddr, err := cfg.Ingest.ListenNetAddr()
	if err != nil {
		return err
	}

	// Starting provider core
	eng, err := engine.New(
		engine.WithDatastore(ds),
		engine.WithDataTransfer(dt),
		engine.WithDirectAnnounce(cfg.DirectAnnounce.URLs...),
		engine.WithHost(h),
		engine.WithEntriesCacheCapacity(cfg.Ingest.LinkCacheSize),
		engine.WithChainedEntries(cfg.Ingest.LinkedChunkSize),
		engine.WithPublisherKind(engine.HttpPublisher),
		engine.WithHttpPublisherListenAddr(httpListenAddr),
		engine.WithHttpPublisherAnnounceAddr(cfg.Ingest.AnnounceMultiaddr),
		engine.WithRetrievalAddrs(cfg.ProviderServer.RetrievalMultiaddrs...),
	)
	if err != nil {
		return err
	}

	err = eng.Start(ctx)
	if err != nil {
		return err
	}

	droutingErrChan := make(chan error, 1)
	// If there are bootstrap peers and bootstrapping is enabled, then try to
	// connect to the minimum set of peers.
	if len(cfg.Bootstrap.Peers) != 0 && cfg.Bootstrap.MinimumPeers != 0 {
		addrs, err := cfg.Bootstrap.PeerAddrs()
		if err != nil {
			return fmt.Errorf("bad bootstrap peer: %s", err)
		}

		bootCfg := bootstrap.BootstrapConfigWithPeers(addrs)
		bootCfg.MinPeerThreshold = cfg.Bootstrap.MinimumPeers

		bootstrapper, err := bootstrap.Bootstrap(peerID, h, nil, bootCfg)
		if err != nil {
			return fmt.Errorf("bootstrap failed: %s", err)
		}
		defer bootstrapper.Close()
	}

	var finalErr error
	// Keep process running.
	select {
	case <-cctx.Done():
	case err = <-droutingErrChan:
		log.Errorw("Failed to start delegated routing server", "err", err)
		finalErr = ErrDaemonStart
	}

	log.Infow("Shutting down daemon")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	go func() {
		// Wait for context to be canceled. If timeout, then exit with error.
		<-shutdownCtx.Done()
		if shutdownCtx.Err() == context.DeadlineExceeded {
			fmt.Println("Timed out on shutdown, terminating...")
			os.Exit(-1)
		}
	}()

	if err = eng.Shutdown(); err != nil {
		log.Errorf("Error closing provider core: %s", err)
		finalErr = ErrDaemonStop
	}

	if err = ds.Close(); err != nil {
		log.Errorf("Error closing provider datastore: %s", err)
		finalErr = ErrDaemonStop
	}

	// cancel libp2p server
	cancelp2p()
	return finalErr
}

// dirWritable checks if a directory is writable. If the directory does
// not exist it is created with writable permission.
func dirWritable(dir string) error {
	if dir == "" {
		return errors.New("directory not specified")
	}

	var err error
	dir, err = homedir.Expand(dir)
	if err != nil {
		return err
	}

	if _, err = os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// dir doesn't exist, check that we can create it
			err = os.Mkdir(dir, 0o775)
			if err == nil {
				return nil
			}
		}
		if errors.Is(err, os.ErrPermission) {
			err = os.ErrPermission
		}
		return fmt.Errorf("cannot write to %s: %w", dir, err)
	}

	// dir exists, make sure we can write to it
	file, err := os.CreateTemp(dir, "test")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			err = os.ErrPermission
		}
		return fmt.Errorf("cannot write to %s: %w", dir, err)
	}
	file.Close()
	return os.Remove(file.Name())
}
