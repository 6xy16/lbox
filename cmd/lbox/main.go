package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/xy/lbox/internal/runtime"
)

// ---------------------------------------------------------------------------
// lbox —— 极简容器运行时 CLI
//
// 命令结构：
//   lbox run [选项] <命令> [参数...]   在新 namespace 中运行命令
//   lbox init <命令> [参数...]         [内部命令] 容器内初始化，不对外使用
// ---------------------------------------------------------------------------

const usage = `lbox —— 手写极简容器运行时（不依赖 Docker / runc）

用法:
  lbox run [选项] <命令> [参数...]   在新 namespace 中运行命令
  lbox ps                           列出所有正在运行的容器
  lbox stop <容器ID>                停止一个运行中的容器
  lbox rm <容器ID>                  删除一个已停止的容器
  lbox prune                        清理所有残留的死容器目录
  lbox exec <容器ID> <命令> [参数]  在运行中的容器里执行命令

选项:
  --cpu <核数>     CPU 配额，单位核。如 --cpu 0.5 表示最多用半个核
  --mem <大小>     内存上限，如 --mem 256M、--mem 1G
  --pids <数量>    容器内最大进程/线程数，如 --pids 64
  --name <id>      指定容器 ID（默认随机生成）
  --rootfs <路径>  指定容器根文件系统目录（启用 pivot_root 文件系统隔离）
  --hostname <名>  设置容器主机名（默认 lbox）
  --network        启用独立网络、bridge、NAT 和动态 IP
  -p <宿主:容器>   配置 TCP 端口映射，如 8080:80（需配合 --network）

示例:
  # 1. 基本隔离：进入一个看不到宿主机进程的 shell
  sudo ./lbox run /bin/sh

  # 2. 限制 0.5 核 + 256M 内存 + 最多 32 个进程
  sudo ./lbox run --cpu 0.5 --mem 256M --pids 32 /bin/sh

  # 3. 完整容器：pivot_root 到 busybox rootfs，完全隔离文件系统
  sudo ./lbox run --rootfs ./rootfs /bin/sh

  # 4. 查看正在运行的容器
  sudo ./lbox ps

说明:
  创建 namespace 需要 CAP_SYS_ADMIN，请用 sudo 运行。
  尚未启用 user namespace，因此不能免 sudo。
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch args[0] {
	// "init" 是内部子命令：父进程通过 /proc/self/exe 重新执行自己时触发。
	// 走到这里说明当前进程已经处于新的 namespace 中了。
	case runtime.InitCommand:
		if err := runtime.Init(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "lbox: %v\n", err)
			os.Exit(1)
		}
		return

	case "run":
		if err := runCommand(args[1:]); err != nil {
			exitWithCommandError(err)
		}

	case "ps":
		if err := psCommand(); err != nil {
			fmt.Fprintf(os.Stderr, "lbox: %v\n", err)
			os.Exit(1)
		}

	case "stop":
		if err := stopCommand(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "lbox: %v\n", err)
			os.Exit(1)
		}

	case "rm":
		if err := rmCommand(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "lbox: %v\n", err)
			os.Exit(1)
		}

	case "prune":
		if err := pruneCommand(); err != nil {
			fmt.Fprintf(os.Stderr, "lbox: %v\n", err)
			os.Exit(1)
		}

	case "exec":
		if err := execCommand(args[1:]); err != nil {
			exitWithCommandError(err)
		}

	default:
		fmt.Fprintf(os.Stderr, "lbox: 未知命令 %q\n\n", args[0])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func exitWithCommandError(err error) {
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	fmt.Fprintf(os.Stderr, "lbox: %v\n", err)
	os.Exit(1)
}

// runCommand 解析 "lbox run" 的参数并启动容器。
func runCommand(argv []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	var (
		cpu      = fs.Float64("cpu", 0, "CPU 配额（核）")
		mem      = fs.String("mem", "", "内存上限，如 256M")
		pids     = fs.Int("pids", 0, "最大进程数")
		name     = fs.String("name", "", "容器 ID")
		host     = fs.String("hostname", runtime.DefaultHostname, "容器主机名")
		rootfs   = fs.String("rootfs", "", "容器根文件系统目录（启用 pivot_root）")
		network  = fs.Bool("network", false, "启用网络隔离（独立 network namespace）")
		publish  = fs.String("p", "", "端口映射，格式: hostPort:containerPort")
		publish2 = fs.String("publish", "", "端口映射，格式: hostPort:containerPort")
	)

	if err := fs.Parse(argv); err != nil {
		return err
	}

	command := fs.Args()
	if len(command) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	// 校验参数，早点报错比在内核那里拿到 EINVAL 友好得多。
	// 注意：flag.Float64 / flag.Int 返回的是指针，比较前必须解引用。
	if *cpu < 0 || math.IsNaN(*cpu) || math.IsInf(*cpu, 0) {
		return fmt.Errorf("--cpu 必须是有限的非负数")
	}
	if *pids < 0 {
		return fmt.Errorf("--pids 不能为负数")
	}

	// 处理端口映射参数
	var portBindings []string
	if *publish != "" {
		portBindings = append(portBindings, *publish)
	}
	if *publish2 != "" {
		portBindings = append(portBindings, *publish2)
	}

	cfg := &runtime.Config{
		ID:            *name,
		Command:       command,
		Hostname:      *host,
		Flags:         runtime.FlagsPhase1,
		Rootfs:        *rootfs,
		EnableNetwork: *network,
		PortBindings:  portBindings,
		Resource: runtime.Resource{
			CPUCores:  *cpu,
			MemoryMax: *mem,
			PidsMax:   *pids,
		},
	}
	return runtime.Run(cfg)
}

// psCommand 列出所有正在运行的容器。
func psCommand() error {
	containers, err := runtime.ListContainers()
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		fmt.Println("没有正在运行的容器")
		return nil
	}

	// 打印表格头
	fmt.Printf("%-20s %-8s %-12s %-12s %-12s\n", "CONTAINER ID", "PID", "CPU", "MEMORY", "PIDS")
	fmt.Printf("%-20s %-8s %-12s %-12s %-12s\n", "------------", "---", "---", "------", "----")

	for _, c := range containers {
		cpu := c.CPUMax
		if cpu == "max" {
			cpu = "无限制"
		}
		mem := c.MemMax
		if mem == "max" {
			mem = "无限制"
		}
		pids := c.PidsMax
		if pids == "max" {
			pids = "无限制"
		}

		fmt.Printf("%-20s %-8d %-12s %-12s %-12s\n",
			c.ID, c.PID, cpu, mem, pids)
	}

	return nil
}

// stopCommand 停止一个运行中的容器。
func stopCommand(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("用法: lbox stop <容器ID>")
	}
	return runtime.Stop(argv[0])
}

// rmCommand 删除一个已停止的容器。
func rmCommand(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("用法: lbox rm <容器ID>")
	}
	return runtime.Remove(argv[0])
}

// pruneCommand 清理所有残留的死容器 cgroup 目录。
func pruneCommand() error {
	removed, err := runtime.RmAllStopped()
	if err != nil {
		return err
	}
	if removed == 0 {
		fmt.Println("没有需要清理的残留容器")
		return nil
	}
	fmt.Printf("已清理 %d 个残留容器\n", removed)
	return nil
}

// execCommand 在运行中的容器里执行命令。
func execCommand(argv []string) error {
	if len(argv) < 2 {
		return fmt.Errorf("用法: lbox exec <容器ID> <命令> [参数...]")
	}
	containerID := argv[0]
	command := argv[1:]
	return runtime.ExecInContainer(containerID, command)
}
