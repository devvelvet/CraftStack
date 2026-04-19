package observability

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// InfluxPusher periodically pushes snapshots to InfluxDB v2 using line protocol
// over the /api/v2/write endpoint. Implemented with net/http to avoid pulling
// in the InfluxDB SDK.
type InfluxPusher struct {
	URL      string
	Token    string
	Org      string
	Bucket   string
	Interval time.Duration
	Source   MetricsSource
	Log      *slog.Logger
	client   *http.Client
}

// NewInfluxPusher builds a pusher with sane defaults.
func NewInfluxPusher(url, token, org, bucket string, interval time.Duration, src MetricsSource, log *slog.Logger) *InfluxPusher {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &InfluxPusher{
		URL:      strings.TrimRight(url, "/"),
		Token:    token,
		Org:      org,
		Bucket:   bucket,
		Interval: interval,
		Source:   src,
		Log:      log,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Run blocks until ctx is canceled, pushing every Interval.
func (p *InfluxPusher) Run(ctx context.Context) {
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	p.Log.Info("influxdb pusher started", "url", p.URL, "bucket", p.Bucket, "interval", p.Interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := p.pushOnce(ctx); err != nil {
				p.Log.Warn("influxdb push failed", "error", err)
			}
		}
	}
}

func (p *InfluxPusher) pushOnce(ctx context.Context) error {
	var buf bytes.Buffer
	now := time.Now().UnixNano()
	for _, n := range p.Source.NodeSnapshots() {
		fmt.Fprintf(&buf,
			"craftstack_node,node=%s cpu_percent=%g,mem_percent=%g,mem_used_mb=%di,mem_total_mb=%di,disk_percent=%g %d\n",
			escapeTag(n.NodeID), n.CPUPercent, n.MemPercent, n.MemUsedMB, n.MemTotalMB, n.DiskPercent, now)
	}
	for _, in := range p.Source.InstanceSnapshots() {
		fmt.Fprintf(&buf,
			"craftstack_instance,instance=%s,node=%s,type=%s cpu_percent=%g,mem_used_mb=%di,mem_limit_mb=%di,net_rx=%di,net_tx=%di,blk_r=%di,blk_w=%di %d\n",
			escapeTag(in.InstanceID), escapeTag(in.NodeID), escapeTag(in.Type),
			in.CPUPercent, in.MemoryUsedMB, in.MemoryLimitMB, in.NetRxBytes, in.NetTxBytes,
			in.BlockReadBytes, in.BlockWriteBytes, now)
	}
	if buf.Len() == 0 {
		return nil
	}

	url := fmt.Sprintf("%s/api/v2/write?org=%s&bucket=%s&precision=ns", p.URL, p.Org, p.Bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+p.Token)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("influxdb status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// escapeTag escapes InfluxDB line-protocol tag values (commas, spaces, equals).
func escapeTag(s string) string {
	r := strings.NewReplacer(",", `\,`, " ", `\ `, "=", `\=`)
	return r.Replace(s)
}
