// stress —— lbox 的资源限制验证工具
//
// 为什么需要它？因为资源限制"看起来生效"和"真的生效"是两回事。
// 这个小程序会自己报告实际拿到了多少 CPU 时间，
// 于是在 --cpu 0.5 的容器里跑，就能直接看到"墙钟 4 秒只拿到 2 秒 CPU"。
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

// cpuSeconds 读取本进程至今消耗的 CPU 时间（用户态 + 内核态）。
// 数据来自 getrusage(2)，内核按 CFS 配额记账，因此被 cgroup 限流后这个数会明显偏小。
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

// burnCPU 空转指定秒数，然后对比「墙钟时间」和「实际拿到的 CPU 时间」。
//
// 没有被限制时：CPU 时间 ≈ 墙钟时间（比值 ≈ 1.0）
// 被 --cpu 0.5 限制时：CPU 时间 ≈ 墙钟时间的一半（比值 ≈ 0.5）
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

// eatMemory 申请并真正"触碰"指定大小的内存。
//
// 关键点：只 malloc 不写入是不够的 —— Linux 是按需分配物理页的，
// 不触碰就不会真正占用内存，也就不会触发 cgroup 的 memory.max。
// 这里每隔一个页（4KB）写 1 字节，强制内核把物理页分配出来。
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
