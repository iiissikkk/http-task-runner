package delivery

import (
	"net/http"

	"todoapp/internal/delivery/http/docs"
	httpopenapi "todoapp/internal/delivery/http/openapi"

	"github.com/gorilla/mux"
)

func NewRouter(handler httpopenapi.StrictServerInterface) http.Handler {
	router := mux.NewRouter()
	strictHandler := httpopenapi.NewStrictHandler(handler, nil)
	httpopenapi.HandlerFromMux(strictHandler, router)
	router.HandleFunc("/swagger", docs.SwaggerSpec).Methods(http.MethodGet)
	router.HandleFunc("/swagger/index.html", docs.SwaggerUI).Methods(http.MethodGet)
	return router
}
