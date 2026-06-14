package widget

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/glanceapp/glance/internal/assets"
)

// ServerStats zeigt CPU, RAM, Disk, Docker und Uptime des Hosts
type ServerStats struct {
	widgetBase `yaml:",inline"`

	// Runtime-Daten (werden in Update() gesetzt)
	CPUUser        int     `yaml:"-"`
	CPUSystem      int     `yaml:"-"`
	CPUIdle        int     `yaml:"-"`
	CPUUserScale   float64 `yaml:"-"`
	CPUSystemScale float64 `yaml:"-"`
	RAMUsedGB      float64 `yaml:"-"`
	RAMTotalGB     float64 `yaml:"-"`
	RAMPercent     int     `yaml:"-"`
	RAMScale       float64 `yaml:"-"`
	DiskUsedGB     float64 `yaml:"-"`
	DiskTotalGB    float64 `yaml:"-"`
	DiskPercent    int     `yaml:"-"`
	DiskScale      float64 `yaml:"-"`
	DockerRunning  int     `yaml:"-"`
	DockerStopped  int     `yaml:"-"`
	DockerTotal    int     `yaml:"-"`
	UptimeHours    int     `yaml:"-"`
	UptimeMinutes  int     `yaml:"-"`
	UptimeDays     int     `yaml:"-"`
}

type serverStatsSnapshot struct {
	CPUUser        int
	CPUSystem      int
	CPUIdle        int
	CPUUserScale   float64
	CPUSystemScale float64
	RAMUsedGB      float64
	RAMTotalGB     float64
	RAMPercent     int
	RAMScale       float64
	DiskUsedGB     float64
	DiskTotalGB    float64
	DiskPercent    int
	DiskScale      float64
	DockerRunning  int
	DockerStopped  int
	DockerTotal    int
	UptimeHours    int
	UptimeMinutes  int
	UptimeDays     int
}

func init() {
	Register("server-stats", func() Widget { return &ServerStats{} })
}

func (widget *ServerStats) Initialize() error {
	widget.withTitle("Server Stats")
	widget.withCacheDuration(30 * time.Second)
	return nil
}

func (widget *ServerStats) Update(ctx context.Context, services ExternalServiceProvider) {
	var snap serverStatsSnapshot
	widget.readCPU(&snap)
	widget.readRAM(&snap)
	widget.readDisk(&snap)
	widget.readDocker(&snap)
	widget.readUptime(&snap)

	widget.Lock()
	widget.CPUUser = snap.CPUUser
	widget.CPUSystem = snap.CPUSystem
	widget.CPUIdle = snap.CPUIdle
	widget.CPUUserScale = snap.CPUUserScale
	widget.CPUSystemScale = snap.CPUSystemScale
	widget.RAMUsedGB = snap.RAMUsedGB
	widget.RAMTotalGB = snap.RAMTotalGB
	widget.RAMPercent = snap.RAMPercent
	widget.RAMScale = snap.RAMScale
	widget.DiskUsedGB = snap.DiskUsedGB
	widget.DiskTotalGB = snap.DiskTotalGB
	widget.DiskPercent = snap.DiskPercent
	widget.DiskScale = snap.DiskScale
	widget.DockerRunning = snap.DockerRunning
	widget.DockerStopped = snap.DockerStopped
	widget.DockerTotal = snap.DockerTotal
	widget.UptimeHours = snap.UptimeHours
	widget.UptimeMinutes = snap.UptimeMinutes
	widget.UptimeDays = snap.UptimeDays
	widget.Unlock()

	widget.canContinueUpdateAfterHandlingErr(nil)
}

func (widget *ServerStats) Render() template.HTML {
	return widget.render(widget, assets.ServerStatsTemplate)
}

// cpuSample represents a snapshot of /proc/stat cpu line
type cpuSample struct {
	user, nice, system, idle, iowait, irq, softirq, steal uint64
}

func (s cpuSample) total() uint64 {
	return s.user + s.nice + s.system + s.idle + s.iowait + s.irq + s.softirq + s.steal
}

func parseCPUSample(data string) cpuSample {
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		if len(fields) < 5 {
			continue
		}
		parse := func(i int) uint64 {
			if i >= len(fields) {
				return 0
			}
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			return v
		}
		return cpuSample{
			user:    parse(0),
			nice:    parse(1),
			system:  parse(2),
			idle:    parse(3),
			iowait:  parse(4),
			irq:     parse(5),
			softirq: parse(6),
			steal:   parse(7),
		}
	}
	return cpuSample{}
}

func calcCPUPercent(prev, curr cpuSample) (user, system, idle int) {
	prevTotal := prev.total()
	currTotal := curr.total()
	if currTotal <= prevTotal {
		return 0, 0, 0
	}
	totalDelta := currTotal - prevTotal
	if totalDelta == 0 {
		return 0, 0, 0
	}
	userDelta := (curr.user - prev.user) + (curr.nice - prev.nice)
	sysDelta := (curr.system - prev.system) + (curr.irq - prev.irq) + (curr.softirq - prev.softirq) + (curr.steal - prev.steal)
	idleDelta := (curr.idle - prev.idle) + (curr.iowait - prev.iowait)

	user = int((userDelta * 100) / totalDelta)
	system = int((sysDelta * 100) / totalDelta)
	idle = int((idleDelta * 100) / totalDelta)
	return
}

// readCPU reads real CPU usage by taking two samples 500ms apart
func (widget *ServerStats) readCPU(snap *serverStatsSnapshot) {
	data1, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	s1 := parseCPUSample(string(data1))

	time.Sleep(500 * time.Millisecond)

	data2, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	s2 := parseCPUSample(string(data2))

	snap.CPUUser, snap.CPUSystem, snap.CPUIdle = calcCPUPercent(s1, s2)
	snap.CPUUserScale = float64(snap.CPUUser) / 100.0
	snap.CPUSystemScale = float64(snap.CPUSystem) / 100.0
}

func (widget *ServerStats) readRAM(snap *serverStatsSnapshot) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	var memTotal, memAvailable, memFree int
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %d kB", &memTotal)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %d kB", &memAvailable)
		} else if strings.HasPrefix(line, "MemFree:") {
			fmt.Sscanf(line, "MemFree: %d kB", &memFree)
		}
	}
	if memTotal <= 0 {
		return
	}
	// Fallback: if MemAvailable is 0 (older kernels), approximate with MemFree + Buffers + Cached
	if memAvailable <= 0 {
		var buffers, cached int
		for _, line := range lines {
			if strings.HasPrefix(line, "Buffers:") {
				fmt.Sscanf(line, "Buffers: %d kB", &buffers)
			} else if strings.HasPrefix(line, "Cached:") {
				fmt.Sscanf(line, "Cached: %d kB", &cached)
			}
		}
		memAvailable = memFree + buffers + cached
	}

	snap.RAMTotalGB = float64(memTotal) / 1024 / 1024
	used := memTotal - memAvailable
	if used < 0 {
		used = 0
	}
	snap.RAMUsedGB = float64(used) / 1024 / 1024
	snap.RAMPercent = (used * 100) / memTotal
	snap.RAMScale = float64(snap.RAMPercent) / 100.0
}

func (widget *ServerStats) readDisk(snap *serverStatsSnapshot) {
	cmd := exec.Command("df", "-B1", "/")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return
	}
	for i := 1; i < len(lines); i++ {
		fields := strings.Fields(lines[i])
		if len(fields) < 6 {
			continue
		}
		// fields[5] is the mount point
		if fields[5] == "/" {
			total, _ := strconv.ParseInt(fields[1], 10, 64)
			used, _ := strconv.ParseInt(fields[2], 10, 64)
			if total > 0 {
				snap.DiskTotalGB = float64(total) / 1024 / 1024 / 1024
				snap.DiskUsedGB = float64(used) / 1024 / 1024 / 1024
				snap.DiskPercent = int((used * 100) / total)
				snap.DiskScale = float64(snap.DiskPercent) / 100.0
			}
			return
		}
	}
}

func (widget *ServerStats) readDocker(snap *serverStatsSnapshot) {
	// Running containers
	cmd := exec.Command("docker", "ps", "-q")
	out, err := cmd.Output()
	if err == nil {
		snap.DockerRunning = countNonEmptyLines(string(out))
	} else {
		snap.DockerRunning = -1
	}

	// All containers
	cmd = exec.Command("docker", "ps", "-aq")
	out, err = cmd.Output()
	if err == nil {
		snap.DockerTotal = countNonEmptyLines(string(out))
	} else {
		snap.DockerTotal = -1
	}

	if snap.DockerRunning >= 0 && snap.DockerTotal >= 0 {
		snap.DockerStopped = snap.DockerTotal - snap.DockerRunning
	} else {
		snap.DockerStopped = -1
	}
}

func countNonEmptyLines(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func (widget *ServerStats) readUptime(snap *serverStatsSnapshot) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return
	}
	seconds, _ := strconv.ParseFloat(fields[0], 64)
	totalMinutes := int(seconds) / 60
	snap.UptimeDays = totalMinutes / 60 / 24
	snap.UptimeHours = (totalMinutes / 60) % 24
	snap.UptimeMinutes = totalMinutes % 60
}
