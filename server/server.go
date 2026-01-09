package server

import (
	"context"
	"golangdb/database"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	http     *http.Server
	Database *database.DB
	Router   *chi.Mux
}

func NewServer(db *database.DB, addr string) *Server {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.StripSlashes)

	s := &Server{
		Database: db,
		Router:   router,
	}

	httpSrv := &http.Server{
		Addr:    ":" + addr,
		Handler: router,
	}

	// TODO: cors go here:

	s.http = httpSrv

	s.Routes()

	return s
}

func (s *Server) Routes() {

	s.Router.Post("/sign-up", s.SingUpHandler)
	s.Router.Post("/login", s.LoginHandler)

	s.Router.Group(func(r chi.Router) {
		r.Use(JWTmiddleware)
		r.Post("/create", s.InsertHandler)
		r.Delete("/delete", s.DeleteHandler)
		r.Get("/get", s.SelectHandler)

		r.Route("/admin", func(r chi.Router) {
			r.Use(AdminOnly)
			r.Get("/getall", s.SelectHandler)
		})
	})
}

func (s *Server) Start() error {
	return s.http.ListenAndServe()
}

func (s *Server) ShutdownGracefully(ctx context.Context) error {
	if err := s.http.Shutdown(ctx); err != nil {
		return err
	}

	return s.Database.Database.Close()
}
