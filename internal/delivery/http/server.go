package delivery

import "net/http"

type Server struct {
	addr   string
	router http.Handler
}

func NewServer(addr string, router http.Handler) *Server {
	return &Server{addr: addr, router: router}
}

func (s *Server) Start() error {
	return http.ListenAndServe(s.addr, s.router)
}
