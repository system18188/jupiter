package main

import (
	"github.com/douyu/jupiter"
	"github.com/douyu/jupiter/pkg/server/xrestful"
	"github.com/douyu/jupiter/pkg/xlog"
	"net/http"
)

func main() {
	eng := NewEngine()
	if err := eng.Run(); err != nil {
		xlog.Panic(err.Error())
	}
}

type Engine struct {
	jupiter.Application
}

func NewEngine() *Engine {
	eng := &Engine{}
	if err := eng.Startup(
		eng.serveHTTP,
	); err != nil {
		xlog.Panic("startup", xlog.Any("err", err))
	}
	return eng
}

// HTTP地址
func (eng *Engine) serveHTTP() error {
	server := xrestful.StdConfig("http").Build()

	ws := server.WebService()

	ws.Path("/").
		Route(ws.GET("").
			To(func(request *restful.Request, response *restful.Response) {
				response.WriteErrorString(http.StatusOK, "hello go-restful")
			}))

	return eng.Serve(server)
}
