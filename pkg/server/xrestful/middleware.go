package xrestful

import (
	"bytes"
	"fmt"
	"github.com/douyu/jupiter/pkg/metric"
	"github.com/douyu/jupiter/pkg/trace"
	"github.com/douyu/jupiter/pkg/xlog"
	"github.com/emicklei/go-restful/v3"
	"go.uber.org/zap"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

var (
	dunno     = []byte("???")
	centerDot = []byte("·")
	dot       = []byte(".")
	slash     = []byte("/")
)

func extractAID(req *restful.Request) string {
	return req.Request.Header.Get("AID")
}

func recoverMiddleware(logger *xlog.Logger, slowQueryThresholdInMilli int64) restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		var beg = time.Now()
		var fields = make([]xlog.Field, 0, 8)
		var brokenPipe bool
		defer func() {
			//Latency
			fields = append(fields, zap.Float64("cost", time.Since(beg).Seconds()))
			if slowQueryThresholdInMilli > 0 {
				if cost := int64(time.Since(beg)) / 1e6; cost > slowQueryThresholdInMilli {
					fields = append(fields, zap.Int64("slow", cost))
				}
			}
			if rec := recover(); rec != nil {
				if ne, ok := rec.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}
				var err = rec.(error)
				fields = append(fields, zap.ByteString("stack", stack(3)))
				fields = append(fields, zap.String("err", err.Error()))
				logger.Error("access", fields...)
				// If the connection is dead, we can't write a status to it.
				if brokenPipe {
					resp.WriteErrorString(http.StatusInternalServerError, err.Error())
					return
				}
				resp.WriteHeader(http.StatusInternalServerError)
				return
			}
			// httpRequest, _ := httputil.DumpRequest(c.Request, false)
			// fields = append(fields, zap.ByteString("request", httpRequest))
			fields = append(fields,
				zap.String("method", req.Request.Method),
				zap.Int("code", resp.StatusCode()),
				//zap.Int("size", req.Writer.Size()),
				zap.String("host", req.Request.Host),
				zap.String("path", req.Request.URL.Path),
				zap.String("ip", clientIP(req)),
			//zap.String("err", req.Errors.ByType(gin.ErrorTypePrivate).String()),
			)
			logger.Info("access", fields...)
		}()

		chain.ProcessFilter(req, resp)
	}
}

// stack returns a nicely formatted stack frame, skipping skip frames.
func stack(skip int) []byte {
	buf := new(bytes.Buffer) // the returned data
	// As we loop, we open files and read them. These variables record the currently
	// loaded file.
	var lines [][]byte
	var lastFile string
	for i := skip; ; i++ { // Skip the expected number of frames
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		// Print this much at least.  If we can't find the source, it won't show.
		fmt.Fprintf(buf, "%s:%d (0x%x)\n", file, line, pc)
		if file != lastFile {
			data, err := ioutil.ReadFile(file)
			if err != nil {
				continue
			}
			lines = bytes.Split(data, []byte{'\n'})
			lastFile = file
		}
		fmt.Fprintf(buf, "\t%s: %s\n", function(pc), source(lines, line))
	}
	return buf.Bytes()
}

// source returns a space-trimmed slice of the n'th line.
func source(lines [][]byte, n int) []byte {
	n-- // in stack trace, lines are 1-indexed but our array is 0-indexed
	if n < 0 || n >= len(lines) {
		return dunno
	}
	return bytes.TrimSpace(lines[n])
}

// function returns, if possible, the name of the function containing the PC.
func function(pc uintptr) []byte {
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return dunno
	}
	name := []byte(fn.Name())
	// The name includes the path name to the package, which is unnecessary
	// since the file name is already included.  Plus, it has center dots.
	// That is, we see
	//	runtime/debug.*T·ptrmethod
	// and want
	//	*T.ptrmethod
	// Also the package path might contains dot (e.g. code.google.com/...),
	// so first eliminate the path prefix
	if lastSlash := bytes.LastIndex(name, slash); lastSlash >= 0 {
		name = name[lastSlash+1:]
	}
	if period := bytes.Index(name, dot); period >= 0 {
		name = name[period+1:]
	}
	name = bytes.Replace(name, centerDot, dot, -1)
	return name
}

func timeFormat(t time.Time) string {
	timeString := t.Format("2006/01/02 - 15:04:05")
	return timeString
}

func metricServerInterceptor() restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		beg := time.Now()
		metric.ServerHandleHistogram.Observe(time.Since(beg).Seconds(), metric.TypeHTTP, req.Request.Method+"."+req.Request.URL.Path, extractAID(req))
		metric.ServerHandleCounter.Inc(metric.TypeHTTP, req.Request.Method+"."+req.Request.URL.Path, extractAID(req), http.StatusText(resp.StatusCode()))

		chain.ProcessFilter(req, resp)
	}
}

func traceServerInterceptor() restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		span, ctx := trace.StartSpanFromContext(
			req.Request.Context(),
			req.Request.Method+" "+req.Request.URL.Path,
			trace.TagComponent("http"),
			trace.TagSpanKind("server"),
			trace.HeaderExtractor(req.Request.Header),
			trace.CustomTag("http.url", req.Request.URL.Path),
			trace.CustomTag("http.method", req.Request.Method),
			trace.CustomTag("peer.ipv4", clientIP(req)),
		)
		req.Request.WithContext(ctx)
		defer span.Finish()

		chain.ProcessFilter(req, resp)
	}
}

// IP returns the IP address of request.
func clientIP(req *restful.Request) string {
	ra := req.Request.RemoteAddr
	if ip := req.HeaderParameter("X-Forwarded-For"); ip != "" {
		ra = strings.Split(ip, ", ")[0]
	} else if ip := req.HeaderParameter("X-Real-IP"); ip != "" {
		ra = ip
	} else {
		ra, _, _ = net.SplitHostPort(ra)
	}
	return ra
}
