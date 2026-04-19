package metrics

import (
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	gopsnet "github.com/shirou/gopsutil/v3/net"
)

// OSMetrics contains OS-level resource metrics.
type OSMetrics struct {
	CPUUsagePercent float64
	MemoryTotalMB   int64
	MemoryUsedMB    int64
	DiskTotalMB     int64
	DiskUsedMB      int64
	NetworkRxBytes  int64
	NetworkTxBytes  int64
}

// Collector gathers OS-level metrics.
type Collector struct {
	log *slog.Logger
}

// NewCollector creates a new metrics collector.
func NewCollector(log *slog.Logger) *Collector {
	return &Collector{log: log}
}

// CollectOS gathers current OS-level metrics.
func (c *Collector) CollectOS() (*OSMetrics, error) {
	m := &OSMetrics{}

	// CPU
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		c.log.Warn("failed to collect CPU metrics", "error", err)
	} else if len(cpuPercent) > 0 {
		m.CPUUsagePercent = cpuPercent[0]
	}

	// Memory
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		c.log.Warn("failed to collect memory metrics", "error", err)
	} else {
		m.MemoryTotalMB = int64(vmStat.Total / 1024 / 1024)
		m.MemoryUsedMB = int64(vmStat.Used / 1024 / 1024)
	}

	// Disk
	diskPath := "/"
	if runtime.GOOS == "windows" {
		diskPath = "C:"
	}
	diskStat, err := disk.Usage(diskPath)
	if err != nil {
		c.log.Warn("failed to collect disk metrics", "error", err)
	} else {
		m.DiskTotalMB = int64(diskStat.Total / 1024 / 1024)
		m.DiskUsedMB = int64(diskStat.Used / 1024 / 1024)
	}

	// Network
	netStats, err := gopsnet.IOCounters(false)
	if err != nil {
		c.log.Warn("failed to collect network metrics", "error", err)
	} else if len(netStats) > 0 {
		m.NetworkRxBytes = int64(netStats[0].BytesRecv)
		m.NetworkTxBytes = int64(netStats[0].BytesSent)
	}

	return m, nil
}

// FormatCPUCores returns the number of logical CPU cores.
func FormatCPUCores() int {
	return runtime.NumCPU()
}

// FormatOSInfo returns a string describing the OS.
func FormatOSInfo() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}
