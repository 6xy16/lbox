// lmon —— lbox 配套的轻量监控工具
//
// 从 /proc 虚拟文件系统读取系统指标，在终端实时展示。
// 不依赖任何第三方库，纯读内核导出的文件。
//
// 用法:
//
//	./bin/lmon              # 实时监控（每秒刷新）
//	./bin/lmon --once       # 只采样一次，打印后退出
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// CPUStats 保存两次采样的 CPU 时间片，用来算差值。
type CPUStats struct {
	User    uint64
	Nice    uint64
	System  uint64
	Idle    uint64
	Iowait  uint64
	Irq     uint64
	Softirq uint64
}

// MemInfo 保存内存使用情况。
type MemInfo struct {
	Total   uint64 // 总内存（KB）
	Free    uint64 // 空闲内存（KB）
	Avail   uint64 // 可用内存（KB）
	Buffers uint64
	Cached  uint64
}

// NetStat 保存网卡收发字节数。
type NetStat struct {
	Iface   string
	RxBytes uint64
	TxBytes uint64
}

func main() {
	once := flag.Bool("once", false, "只采样一次")
	flag.Parse()

	if *once {
		printSnapshot()
		return
	}

	// 实时监控模式：每秒刷新
	prevCPU, err := readCPU()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lmon: %v\n", err)
		os.Exit(1)
	}
	prevNet := readNet()

	// 清屏
	fmt.Print("\033[2J\033[H")

	for {
		time.Sleep(1 * time.Second)

		curCPU, err := readCPU()
		if err != nil {
			fmt.Fprintf(os.Stderr, "lmon: %v\n", err)
			os.Exit(1)
		}
		curNet := readNet()
		mem := readMem()

		// 计算 CPU 使用率
		cpuUsage := calcCPUUsage(prevCPU, curCPU)

		// 清屏 + 光标回原点
		fmt.Print("\033[2J\033[H")

		// 打印仪表盘
		printHeader()
		printCPU(cpuUsage)
		printMem(mem)
		printNet(curNet, prevNet)
		printFooter()

		prevCPU = curCPU
		prevNet = curNet
	}
}

// readCPU 从 /proc/stat 读 CPU 时间片。
func readCPU() (CPUStats, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return CPUStats{}, err
	}
	// 只取 "cpu "（汇总行），避免误匹配 "cpu0" 等。
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		var cpu CPUStats
		_, err := fmt.Sscanf(line, "cpu %d %d %d %d %d %d %d",
			&cpu.User, &cpu.Nice, &cpu.System, &cpu.Idle,
			&cpu.Iowait, &cpu.Irq, &cpu.Softirq)
		if err != nil {
			return CPUStats{}, fmt.Errorf("解析 /proc/stat 失败: %w", err)
		}
		return cpu, nil
	}
	return CPUStats{}, fmt.Errorf("/proc/stat 中找不到 cpu 汇总行")
}

// calcCPUUsage 计算两次采样之间的 CPU 使用率（百分比）。
func calcCPUUsage(prev, cur CPUStats) float64 {
	prevTotal := prev.User + prev.Nice + prev.System + prev.Idle + prev.Iowait + prev.Irq + prev.Softirq
	curTotal := cur.User + cur.Nice + cur.System + cur.Idle + cur.Iowait + cur.Irq + cur.Softirq

	totalDiff := curTotal - prevTotal
	idleDiff := cur.Idle - prev.Idle

	if totalDiff == 0 {
		return 0
	}

	return float64(totalDiff-idleDiff) / float64(totalDiff) * 100
}

// readMem 从 /proc/meminfo 读内存信息。
func readMem() MemInfo {
	file, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemInfo{}
	}
	var mem MemInfo
	for _, line := range strings.Split(string(file), "\n") {
		var key string
		var val uint64
		var unit string
		if _, err := fmt.Sscanf(line, "%s %d %s", &key, &val, &unit); err != nil {
			continue
		}
		switch strings.TrimSuffix(key, ":") {
		case "MemTotal":
			mem.Total = val
		case "MemFree":
			mem.Free = val
		case "MemAvailable":
			mem.Avail = val
		case "Buffers":
			mem.Buffers = val
		case "Cached":
			mem.Cached = val
		}
	}
	return mem
}

// readNet 从 /proc/net/dev 读网卡流量。
func readNet() []NetStat {
	file, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}

	lines := strings.Split(string(file), "\n")
	var stats []NetStat
	for i := 2; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 10 {
			continue
		}
		var s NetStat
		s.Iface = strings.TrimSuffix(parts[0], ":")
		_, _ = fmt.Sscanf(parts[1], "%d", &s.RxBytes)
		_, _ = fmt.Sscanf(parts[9], "%d", &s.TxBytes)
		stats = append(stats, s)
	}
	return stats
}

// printSnapshot 打印一次采样结果（--once 模式）。
func printSnapshot() {
	mem := readMem()
	cpu, err := readCPU()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lmon: %v\n", err)
		os.Exit(1)
	}
	net := readNet()

	fmt.Println("=== lmon 快照 ===")
	fmt.Printf("CPU: user=%d system=%d idle=%d\n", cpu.User, cpu.System, cpu.Idle)
	fmt.Printf("内存: 总=%dMB 可用=%dMB 已用=%dMB\n",
		mem.Total/1024, mem.Avail/1024, (mem.Total-mem.Avail)/1024)
	fmt.Println("网卡:")
	for _, n := range net {
		fmt.Printf("  %s: rx=%d tx=%d\n", n.Iface, n.RxBytes, n.TxBytes)
	}
}

// printHeader 打印标题。
func printHeader() {
	fmt.Println("┌──────────────────────────────────────────┐")
	fmt.Println("│  lmon — lbox 系统监控仪表盘               │")
	fmt.Println("└──────────────────────────────────────────┘")
	fmt.Println()
}

// printCPU 打印 CPU 使用情况。
func printCPU(usage float64) {
	bar := makeBar(usage, 40)
	fmt.Printf("CPU 使用率: %6.2f%%  [%s]\n", usage, bar)
	fmt.Println()
}

// printMem 打印内存使用情况。
func printMem(mem MemInfo) {
	if mem.Total == 0 {
		fmt.Println("内存: 无法读取")
		fmt.Println()
		return
	}
	used := mem.Total - mem.Avail
	usedMB := used / 1024
	totalMB := mem.Total / 1024
	percent := float64(used) / float64(mem.Total) * 100
	bar := makeBar(percent, 40)

	fmt.Printf("内存: %dMB / %dMB  (%5.1f%%)\n", usedMB, totalMB, percent)
	fmt.Printf("  缓冲: %dMB  缓存: %dMB\n", mem.Buffers/1024, mem.Cached/1024)
	fmt.Printf("  [%s]\n", bar)
	fmt.Println()
}

// printNet 打印网络流量。按网卡名配对，避免顺序变化或增删导致错位。
func printNet(cur, prev []NetStat) {
	prevByIface := make(map[string]NetStat, len(prev))
	for _, s := range prev {
		prevByIface[s.Iface] = s
	}
	fmt.Println("网络:")
	for _, s := range cur {
		p, ok := prevByIface[s.Iface]
		if !ok || s.RxBytes < p.RxBytes || s.TxBytes < p.TxBytes {
			continue // 首次出现或计数器被重置
		}
		rxDiff := s.RxBytes - p.RxBytes
		txDiff := s.TxBytes - p.TxBytes
		fmt.Printf("  %-8s ↓ %6.1f KB/s  ↑ %6.1f KB/s\n",
			s.Iface,
			float64(rxDiff)/1024,
			float64(txDiff)/1024)
	}
	fmt.Println()
}

// printFooter 打印底部提示。
func printFooter() {
	fmt.Println("──────────────────────────────────────────")
	fmt.Printf("刷新时间: %s  (按 Ctrl+C 退出)\n", time.Now().Format("15:04:05"))
}

// makeBar 生成一个文本进度条。
func makeBar(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}
	return fmt.Sprintf("%s%s",
		repeat("█", filled),
		repeat("░", width-filled))
}

// repeat 重复字符串 n 次。
func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
