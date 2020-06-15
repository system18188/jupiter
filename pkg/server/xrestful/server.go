package xrestful

import (
	"context"
	"github.com/emicklei/go-restful"
	"github.com/system18188/jupiter/pkg"
	"github.com/system18188/jupiter/pkg/server"
	"github.com/system18188/jupiter/pkg/xlog"
	"net/http"
)
// Server ...
type Server struct {
	*restful.Container
	Server *http.Server
	config *Config
}

func newServer(config *Config) *Server {
	return &Server{
		Container:   restful.NewContainer(),
		config: config,
	}
}

// Server implements server.Server interface.
func (s *Server) Serve() error {
	s.EnableContentEncoding(s.config.EnableContentEncoding)
	for _, service := range s.Container.RegisteredWebServices() {
		for _, route := range service.Routes() {
			s.config.logger.Info("echo add route", xlog.FieldMethod(route.Method), xlog.String("path", route.Path))
		}
	}
	s.Server = &http.Server{
		Addr:    s.config.Address(),
		Handler: s,
	}
	return s.Server.ListenAndServe()
}

// Stop implements server.Server interface
// it will terminate go-restful server immediately
func (s *Server) Stop() error {
	return s.Server.Close()
}

// GracefulStop implements server.Server interface
// it will stop go-restful server gracefully
func (s *Server) GracefulStop(ctx context.Context) error {
	return s.Server.Shutdown(ctx)
}

// Info returns server info, used by governor and consumer balancer
// TODO(gorexlv): implements government protocol with juno
func (s *Server) Info() *server.ServiceInfo {
	return &server.ServiceInfo{
		Name:      pkg.Name(),
		Scheme:    "http",
		IP:        s.config.Host,
		Port:      s.config.Port,
		Weight:    0.0,
		Enable:    false,
		Healthy:   false,
		Metadata:  map[string]string{},
		Region:    "",
		Zone:      "",
		GroupName: "",
	}
}
