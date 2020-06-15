package xrestful

import (
	"net/http"
	"strings"
)

// GetRemoteAddr returns remote address of the context.
func GetRemoteAddr(c *http.Request) string {
	ret := c.Header.Get("X-forwarded-for")
	ret = strings.TrimSpace(ret)
	if "" == ret {
		ret = c.Header.Get("X-Real-IP")
	}
	ret = strings.TrimSpace(ret)
	if "" == ret {
		return c.RemoteAddr
	}

	return strings.Split(ret, ",")[0]
}
