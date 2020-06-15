// Copyright 2020 Douyu
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package xrestful

import (
	"fmt"
	"runtime"
	"time"

	"github.com/douyu/jupiter/pkg/metric"
	"github.com/douyu/jupiter/pkg/trace"

	"github.com/douyu/jupiter/pkg/xlog"
	"github.com/emicklei/go-restful"
	"go.uber.org/zap"
)

func (s *Config) extractAID(c restful.Request) string {
	return c.Request.Header.Get("AID")
}

// RecoverMiddleware ...
func (invoker *Config) recoverMiddleware() restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		var beg = time.Now()
		var fields = make([]xlog.Field, 0, 8)
		var err error
		defer func() {
			fields = append(fields, zap.Float64("cost", time.Since(beg).Seconds()))
			if rec := recover(); rec != nil {
				switch rec := rec.(type) {
				case error:
					err = rec
				default:
					err = fmt.Errorf("%v", rec)
				}
				stack := make([]byte, 4096)
				length := runtime.Stack(stack, true)
				fields = append(fields, zap.ByteString("stack", stack[:length]))
			}
			fields = append(fields,
				zap.String("method", req.Request.Method),
				zap.Int("code", resp.StatusCode()),
				zap.String("host", req.Request.Host),
			)
			if invoker.SlowQueryThresholdInMilli > 0 {
				if cost := int64(time.Since(beg)) / 1e6; cost > invoker.SlowQueryThresholdInMilli {
					fields = append(fields, zap.Int64("slow", cost))
				}
			}
			if err != nil {
				fields = append(fields, zap.String("err", err.Error()))
				invoker.logger.Error("access", fields...)
				return
			}
			invoker.logger.Info("access", fields...)
		}()

		chain.ProcessFilter(req, resp)
	}
}

func (invoker *Config) metricServerInterceptor() restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		beg := time.Now()
		metric.ServerMetricsHandler.GetHandlerHistogram().
			WithLabelValues(metric.TypeServerHttp, req.Request.Method+"."+req.SelectedRoutePath(), invoker.extractAID(*req)).Observe(time.Since(beg).Seconds())
		metric.ServerMetricsHandler.GetHandlerCounter().
			WithLabelValues(metric.TypeServerHttp, req.Request.Method+"."+req.SelectedRoutePath(), invoker.extractAID(*req), statusText[resp.StatusCode()]).Inc()
		chain.ProcessFilter(req, resp)
	}
}

func (invoker *Config) traceServerInterceptor() restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		span, ctx := trace.StartSpanFromContext(
			req.Request.Context(),
			req.Request.Method+" "+req.SelectedRoutePath(),
			trace.TagComponent("http"),
			trace.TagSpanKind("server"),
			trace.HeaderExtractor(req.Request.Header),
			trace.CustomTag("http.url", req.SelectedRoutePath()),
			trace.CustomTag("http.method", req.Request.Method),
			trace.CustomTag("peer.ipv4", GetRemoteAddr(req.Request)),
		)
		req.Request.WithContext(ctx)
		defer span.Finish()
		chain.ProcessFilter(req, resp)
	}
}
