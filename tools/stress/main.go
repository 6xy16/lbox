// stress —— lbox 的资源限制验证工具。
// 自己报告实际拿到的 CPU 时间 / 内存，用于验证 cgroup 限制是否真的生效。
//
// 用法:
//
//	stress --mode cpu --secs 4     空转 4 秒，报告实际获得的 CPU 时间
//	stress --mode mem --mb 512     吃掉 512MB 内存并保持 5 秒
package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"
	"time"
)

func main() {
	mode := flag.String("mode", "cpu", "cpu 或 mem")
	secs := flag.Float64("secs", 4, "持续秒数")
	mb := flag.Int("mb", 256, "要吃掉的内存 MB 数")
	flag.Parse()

	switch *mode {
	case "cpu":
		burnCPU(*secs)
	case "mem":
		eatMemory(*mb, *secs)
	default:
		fmt.Fprintf(os.Stderr, "未知 mode: %s（只支持 cpu / mem）\n", *mode)
		os.Exit(2)
	}
}

// cpuSeconds 读取本进程累计 CPU 时间（用户态+内核态）。
// 来自 getrusage(2)，被 cgroup 限流后这个数会明显偏小。
func cpuSeconds() float64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	sec := func(t syscall.Timeval) float64 {
		return float64(t.Sec) + float64(t.Usec)/1e6
	}
	return sec(ru.Utime) + sec(ru.Stime)
}

// burnCPU 空转指定秒数，对比墙钟时间与实际 CPU 时间。
// 未限制时 CPU 时间 ≈ 墙钟时间；--cpu 0.5 时约为一半。
func burnCPU(secs float64) {
	start := time.Now()
	cpu0 := cpuSeconds()

	var n uint64
	for time.Since(start).Seconds() < secs {
		n++
	}

	wall := time.Since(start).Seconds()
	cpu := cpuSeconds() - cpu0
	ratio := 0.0
	if wall > 0 {
		ratio = cpu / wall
	}

	fmt.Printf("墙钟 %.2fs，实际 CPU 时间 %.2fs  →  平均占用 %.2f 核 (迭代 %d 次)\n",
		wall, cpu, ratio, n)
	fmt.Printf("提示: 若设置了 --cpu 0.5，上面的核数应接近 0.50 而不是 1.00\n")
}

// eatMemory 申请并真正触碰指定大小的内存。
// 只 malloc 不写入不会真正占用物理页（Linux 按需分配），
// 这里每隔一页写 1 字节强制分配，才会触发 cgroup 的 memory.max。
func eatMemory(mb int, hold float64) {
	const pageSize = 4096
	chunk := make([]byte, 1024*1024) // 1MB

	var blocks [][]byte
	allocated := 0

	for i := 0; i < mb; i++ {
		b := make([]byte, len(chunk))
		// 触碰每一页，触发实际的物理内存分配。
		for off := 0; off < len(b); off += pageSize {
			b[off] = 1
		}
		blocks = append(blocks, b)
		allocated++

		if allocated%64 == 0 {
			fmt.Printf("已分配 %d MB ...\n", allocated)
		}
	}

	fmt.Printf("成功占用 %d MB 内存，保持 %.0f 秒\n", allocated, hold)
	time.Sleep(time.Duration(hold * float64(time.Second)))
	fmt.Printf("未被 OOM 杀掉 —— 说明内存限制 ≥ %d MB\n", allocated)
	_ = blocks // 防止被编译器优化掉
}
