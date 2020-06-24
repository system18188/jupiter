package xrestful

import (
	"context"
	"github.com/douyu/jupiter/pkg"
	"github.com/douyu/jupiter/pkg/ecode"
	"github.com/douyu/jupiter/pkg/server"
	"github.com/douyu/jupiter/pkg/xlog"
	restful "github.com/emicklei/go-restful/v3"
	"net"
	"net/http"
)

// Server ...
type Server struct {
	*restful.Container
	Server   *http.Server
	config   *Config
	listener net.Listener
}

func newServer(config *Config) *Server {
	listener, err := net.Listen("tcp", config.Address())
	if err != nil {
		config.logger.Panic("new go-restful server err", xlog.FieldErrKind(ecode.ErrKindListenErr), xlog.FieldErr(err))
	}
	config.Port = listener.Addr().(*net.TCPAddr).Port
	return &Server{
		Container: restful.NewContainer(),
		config:    config,
		listener:  listener,
	}
}

// Serve implements server.Server interface.
func (s *Server) Serve() error {
	s.Router(restful.CurlyRouter{})

	for _, ws := range s.RegisteredWebServices() {
		for _, route := range ws.Routes() {
			s.config.logger.Info("add route", xlog.FieldMethod(route.Method), xlog.String("path", route.Path))
		}

	}

	s.Server = &http.Server{
		Addr:    s.config.Address(),
		Handler: s,
	}
	err := s.Server.Serve(s.listener)
	if err == http.ErrServerClosed {
		s.config.logger.Info("close go-restful", xlog.FieldAddr(s.config.Address()))
		return nil
	}

	return err
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

func (s *Server) WebService() *restful.WebService {
	ws := new(restful.WebService)

	s.Container.Add(ws)
	return ws
}
