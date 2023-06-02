package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"net/http"

	blocks "github.com/ipfs/go-block-format"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipni/go-libipni/metadata"
	"github.com/ipni/index-provider/engine"
)

type ControlServer struct {
	server *http.Server
	l      net.Listener
	bs     blockstore.Blockstore
	eng    *engine.Engine
}

func NewControlServer(bs blockstore.Blockstore, eng *engine.Engine, listenAddr string) *ControlServer {
	mux := http.NewServeMux()
	server := &http.Server{
		Handler: mux,
	}
	l, _ := net.Listen("tcp", listenAddr)
	s := &ControlServer{server, l, bs, eng}
	mux.HandleFunc("/new", s.handler)

	return s
}

func (s *ControlServer) Start() error {
	log.Infow("admin http server listening", "addr", s.l.Addr())
	return s.server.Serve(s.l)
}

func (s *ControlServer) Shutdown(ctx context.Context) error {
	log.Info("admin http server shutdown")
	return s.server.Shutdown(ctx)
}

func (s *ControlServer) handler(w http.ResponseWriter, r *http.Request) {
	// make a new cid.
	buf := make([]byte, 1024)
	rand.Read(buf)
	blk := blocks.NewBlock(buf)
	if err := s.bs.Put(r.Context(), blk); err != nil {
		w.WriteHeader(500)
		w.Write([]byte(fmt.Sprintf("Could not write block: %v", err)))
		return
	}

	// make an advert for it.
	md := metadata.Default.New(metadata.Bitswap{})
	s.eng.NotifyPut(r.Context(), nil, blk.Cid().Bytes(), md)

	// return the cid.
	w.Write([]byte(blk.Cid().String()))
}
