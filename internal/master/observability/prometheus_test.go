package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSource struct {
	nodes []NodeSnapshot
	insts []InstanceSnapshot
}

func (f *fakeSource) NodeSnapshots() []NodeSnapshot         { return f.nodes }
func (f *fakeSource) InstanceSnapshots() []InstanceSnapshot { return f.insts }

func TestPrometheusHandler(t *testing.T) {
	src := &fakeSource{
		nodes: []NodeSnapshot{{NodeID: "node-a", CPUPercent: 42.5, MemPercent: 70, MemUsedMB: 1024, MemTotalMB: 2048, DiskPercent: 55, UpdatedAt: time.Now()}},
		insts: []InstanceSnapshot{{InstanceID: "inst-1", NodeID: "node-a", Type: "minecraft", CPUPercent: 10, MemoryUsedMB: 512, MemoryLimitMB: 1024, NetRxBytes: 100, NetTxBytes: 200, BlockReadBytes: 300, BlockWriteBytes: 400}},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	PrometheusHandler(src).ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	body := rr.Body.String()
	wants := []string{
		`craftstack_node_cpu_percent{node="node-a"} 42.5`,
		`craftstack_node_memory_used_mb{node="node-a"} 1024`,
		`craftstack_instance_cpu_percent{instance="inst-1",node="node-a",type="minecraft"} 10`,
		`craftstack_instance_net_rx_bytes{instance="inst-1",node="node-a",type="minecraft"} 100`,
		`# HELP craftstack_node_cpu_percent`,
		`# TYPE craftstack_instance_net_rx_bytes counter`,
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing: %q\n--- body ---\n%s", w, body)
		}
	}
}

func TestPrometheusHandlerEmpty(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	PrometheusHandler(&fakeSource{}).ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "# HELP") {
		t.Errorf("expected HELP lines even with no data")
	}
}
