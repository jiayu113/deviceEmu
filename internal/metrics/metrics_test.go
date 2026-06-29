package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 注册过程(包级 var 初始化 + init)不 panic、Handler 返回 200 且含指标名
func TestHandlerServesOurMetrics(t *testing.T) {
	// 先动一下指标,确保有样本输出
	DevicesOnline.Set(3)
	TelemetryPublished.Inc()
	CommandsReceived.WithLabelValues("call", "ok").Inc()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"deviceemu_devices_online",
		"deviceemu_telemetry_published_total",
		"deviceemu_commands_received_total",
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics missing %q", want)
		}
	}
}
