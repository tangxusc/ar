package web

import (
	"context"
	"net/http"

	"github.com/sirupsen/logrus"
	"github.com/tangxusc/ar/backend/pkg/graph"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
)

func Start(ctx context.Context) error {
	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{},
	}))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	r := gin.Default()
	r.Any("/graphql", gin.WrapH(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		srv.ServeHTTP(w, req)
	})))

	r.GET("/", gin.WrapH(playground.Handler("ar GraphQL", "/graphql")))

	go func() {
		if err := r.Run(":" + webServerPort); err != nil {
			logrus.Errorf("Failed to start web server: %v", err)
		}
	}()
	return nil
}
