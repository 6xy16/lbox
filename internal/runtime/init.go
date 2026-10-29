package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// 子进程侧：容器内 PID 1 的初始化和事件循环。
const (
	msNosuid  uintptr = 1
	msNodev   uintptr = 4
	msNoexec  uintptr = 8
	msRec     uintptr = 16384
	msPrivate uintptr = 1 << 18
)

// Init 只能由 Run 通过 clone(CLONE_NEWPID) 创建的内部进程调用。
func Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("init: 缺少要执行的命令")
	}
	// 防止误执行 "lbox init" 时直接修改宿主机挂载传播和 hostname。
	if os.Getenv("LBOX_INIT") != "1" || os.Getpid() != 1 {
		return fmt.Errorf("init 是内部命令，只能由 lbox run 在新 PID namespace 中启动")
	}

	if err := makeMountsPrivate(); err != nil {
		return fmt.Errorf("init: 设置挂载传播为 private 失败: %w", err)
	}

	// pivot_root 前再幂等写入一次，之后 cgroup 文件系统可能已不可见。
	if cgPath := os.Getenv("LBOX_CGROUP_PATH"); cgPath != "" {
		procsFile := cgPath + "/cgroup.procs"
		if err := os.WriteFile(procsFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "[lbox] 警告: 容器内写 cgroup.procs 失败: %v\n", err)
		}
	}

	if rootfs := os.Getenv("LBOX_ROOTFS"); rootfs != "" {
		fmt.Fprintf(os.Stderr, "[lbox] pivot_root 到 %s\n", rootfs)
		if err := SetupRootfs(rootfs); err != nil {
			return fmt.Errorf("init: pivot_root 失败: %w", err)
		}
	} else if err := mountProc(); err != nil {
		return fmt.Errorf("init: 挂载 /proc 失败: %w", err)
	}

	hostname := os.Getenv("LBOX_HOSTNAME")
	if hostname == "" {
		hostname = DefaultHostname
	}
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		return fmt.Errorf("init: 设置 hostname 失败: %w", err)
	}

	if containerIP := os.Getenv("LBOX_CONTAINER_IP"); containerIP != "" {
		// 等父进程创建并移动 veth；EOF 表示父进程配置失败。
		if err := waitForNetworkReady(); err != nil {
			return err
		}
		if err := configureContainerNetwork(containerIP); err != nil {
			return fmt.Errorf("init: 配置容器网络失败: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[lbox] 容器内网络已配置: IP=%s\n", containerIP)
	}

	path, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("init: 找不到命令 %q: %w", args[0], err)
	}
	env := append(os.Environ(), "LBOX_CONTAINER=1", "PS1=(lbox) \\w \\$ ")

	// 必须先注册 SIGCHLD 再启动子进程，否则快速退出时可能丢信号并永久等待。
	sigCh := make(chan os.Signal, 16)
	signal.Notify(sigCh, syscall.SIGCHLD, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	fmt.Fprintf(os.Stderr, "[lbox] 容器 init 就绪 (pid=%d)，启动子进程: %v\n", os.Getpid(), args)
	child, err := os.StartProcess(path, args, &os.ProcAttr{
		Env:   env,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys:   &syscall.SysProcAttr{Setpgid: true},
	})
	if err != nil {
		return fmt.Errorf("init: 启动用户命令失败: %w", err)
	}

	for sig := range sigCh {
		switch sig {
		case syscall.SIGCHLD:
			for {
				var status syscall.WaitStatus
				pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
				if err != nil || pid <= 0 {
					break
				}
				if pid != child.Pid {
					continue // 回收其他已退出的后代
				}

				// 主命令结束后容器也随之结束，清理其进程组里的后台进程。
				_ = syscall.Kill(-child.Pid, syscall.SIGKILL)
				for {
					var ignored syscall.WaitStatus
					pid, err := syscall.Wait4(-1, &ignored, syscall.WNOHANG, nil)
					if err != nil || pid <= 0 {
						break
					}
				}
				os.Exit(waitStatusExitCode(status))
			}
		case syscall.SIGTERM, syscall.SIGINT:
			// 转发给整个用户命令进程组，而不只是直接子进程。
			if err := syscall.Kill(-child.Pid, sig.(syscall.Signal)); err != nil {
				_ = child.Signal(sig)
			}
		}
	}
	return nil
}

func waitForNetworkReady() error {
	fd := os.NewFile(uintptr(3), "network-ready")
	if fd == nil {
		return fmt.Errorf("init: 网络同步文件描述符无效")
	}
	defer fd.Close()
	var ready [1]byte
	n, err := fd.Read(ready[:])
	if err != nil || n != 1 {
		return fmt.Errorf("init: 等待宿主机网络配置失败: %v", err)
	}
	return nil
}

func configureContainerNetwork(containerIP string) error {
	_, vethContainer, err := vethNames(containerIP)
	if err != nil {
		return err
	}
	commands := [][]string{
		{"link", "set", "lo", "up"},
		{"link", "set", vethContainer, "down"},
		{"link", "set", vethContainer, "name", "eth0"},
		{"link", "set", "eth0", "up"},
		{"addr", "add", containerIP + "/24", "dev", "eth0"},
		{"route", "add", "default", "via", "10.0.0.1"},
	}
	for _, args := range commands {
		if output, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("ip %v: %w: %s", args, err, string(output))
		}
	}
	return nil
}

// makeMountsPrivate 等价于 mount --make-rprivate /。
func makeMountsPrivate() error {
	return syscall.Mount("", "/", "", msPrivate|msRec, "")
}

func mountProc() error {
	if _, err := os.Stat("/proc"); err != nil {
		return fmt.Errorf("/proc 不存在: %w", err)
	}
	return syscall.Mount("proc", "/proc", "proc", msNosuid|msNodev|msNoexec, "")
}
