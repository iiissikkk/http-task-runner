package delivery

import (
	"context"
	"net/http"
	"time"
)

type Server struct {
	server *http.Server
}

type ServerConfig struct {
	Addr         string
	Handler      http.Handler
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func NewServer(cfg ServerConfig) *Server {
	return &Server{
		server: &http.Server{
			Addr:         cfg.Addr,
			Handler:      cfg.Handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
	}
}

func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
