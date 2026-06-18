package metrics

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Serve 在 addr 地址启动一个独立的 HTTP 服务器，仅提供 /metrics 端点。
// 调用后立即返回；当 ctx 被取消时服务器自动关闭。
// 如果 addr 为空字符串则不执行任何操作。
func Serve(ctx context.Context, addr string) {
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()
	go srv.ListenAndServe() //nolint:errcheck
}
