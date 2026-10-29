package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// ---------------------------------------------------------------------------
// 父进程侧：通过 /proc/self/exe 自我重执行，在新 namespace 中运行 init。
// ---------------------------------------------------------------------------

const (
	SelfExecPath    = "/proc/self/exe"
	InitCommand     = "init"
	DefaultHostname = "lbox"
)

// ExitError 把用户命令的退出码交还给 CLI，同时允许 Run 的 defer 正常清理。
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("容器进程退出，退出码 %d", e.Code)
}

func (e *ExitError) ExitCode() int {
	return e.Code
}

// Config 描述一次容器运行的参数。
type Config struct {
	ID            string
	Command       []string
	Hostname      string
	Flags         NamespaceFlags
	Resource      Resource
	Rootfs        string
	EnableNetwork bool
	ContainerIP   string
	PortBindings  []string
	Tty           bool
}

// Run 克隆出一个带隔离命名空间的子进程并等待它结束。
func Run(cfg *Config) error {
	if cfg == nil || len(cfg.Command) == 0 {
		return fmt.Errorf("run: 没有指定要执行的命令")
	}
	if cfg.Hostname == "" {
		cfg.Hostname = DefaultHostname
	}
	if cfg.Flags == 0 {
		cfg.Flags = FlagsPhase1
	}
	if cfg.EnableNetwork {
		cfg.Flags |= NamespaceFlags(CloneNewNET)
	}

	// rootfs 中的命令只需在切根后存在，不能拿宿主机 PATH 错杀它。
	if cfg.Rootfs == "" {
		if _, err := exec.LookPath(cfg.Command[0]); err != nil {
			return fmt.Errorf("run: 找不到命令 %q: %w", cfg.Command[0], err)
		}
	}
	if cfg.ID == "" {
		cfg.ID = generateID()
	}
	if err := ValidateContainerID(cfg.ID); err != nil {
		return err
	}
	if !cfg.EnableNetwork && len(cfg.PortBindings) > 0 {
		return fmt.Errorf("端口映射需要同时指定 --network")
	}
	var bindings []portBinding
	for _, raw := range cfg.PortBindings {
		binding, err := parsePortBinding(raw)
		if err != nil {
			return err
		}
		bindings = append(bindings, binding)
	}

	cg := NewCgroup(cfg.ID)
	if err := cg.Create(); err != nil {
		return err
	}
	defer func() {
		if err := cg.Remove(); err != nil {
			fmt.Fprintf(os.Stderr, "[lbox] 警告: 清理 cgroup 失败: %v\n", err)
		}
	}()
	if !cfg.Resource.IsEmpty() {
		if err := cg.Apply(cfg.Resource); err != nil {
			return err
		}
	}

	// IP 必须在 Start 前分配并写入环境；启动后再改 cmd.Env 不会影响子进程。
	var configuredPorts []portBinding
	if cfg.EnableNetwork {
		containerIP, err := AllocateIP()
		if err != nil {
			return fmt.Errorf("分配容器 IP 失败: %w", err)
		}
		cfg.ContainerIP = containerIP
		defer func() {
			for _, binding := range configuredPorts {
				if err := TeardownPortForward(containerIP, binding.hostPort, binding.containerPort); err != nil {
					fmt.Fprintf(os.Stderr, "[lbox] 警告: 清除端口映射失败: %v\n", err)
				}
			}
			if err := ReleaseIP(containerIP); err != nil {
				fmt.Fprintf(os.Stderr, "[lbox] 警告: 释放 IP 失败: %v\n", err)
			}
		}()
	}

	args := append([]string{InitCommand}, cfg.Command...)
	cmd := exec.Command(SelfExecPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: cfg.Flags.Uintptr()}
	cmd.Env = append(os.Environ(),
		"LBOX_INIT=1",
		"LBOX_HOSTNAME="+cfg.Hostname,
		"LBOX_CGROUP_PATH="+cg.Path,
	)
	if cfg.Rootfs != "" {
		cmd.Env = append(cmd.Env, "LBOX_ROOTFS="+cfg.Rootfs)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 父进程通过管道通知 init“网络已就绪”，避免用户命令与网络配置并发执行。
	var networkReader, networkWriter *os.File
	if cfg.EnableNetwork {
		var err error
		networkReader, networkWriter, err = os.Pipe()
		if err != nil {
			return fmt.Errorf("创建网络同步管道失败: %w", err)
		}
		cmd.ExtraFiles = []*os.File{networkReader} // 子进程中固定为 fd 3
		cmd.Env = append(cmd.Env,
			"LBOX_CONTAINER_IP="+cfg.ContainerIP,
			"LBOX_NET_READY_FD=3",
		)
	}

	printBanner(cfg)
	if err := cmd.Start(); err != nil {
		if networkReader != nil {
			_ = networkReader.Close()
			_ = networkWriter.Close()
		}
		return fmt.Errorf("启动容器失败: %w", err)
	}
	if networkReader != nil {
		_ = networkReader.Close()
	}

	// Start 后、Wait 前的失败都要杀掉并回收 init，不能留下等待管道的孤儿进程。
	abort := func(runErr error) error {
		if networkWriter != nil {
			_ = networkWriter.Close()
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return runErr
	}

	if err := cg.AddProcess(cmd.Process.Pid); err != nil {
		return abort(fmt.Errorf("把进程加入 cgroup 失败: %w", err))
	}
	fmt.Fprintf(os.Stderr, "[lbox] 已把 pid=%d 加入 cgroup %s\n", cmd.Process.Pid, cg.Path)
	if !cfg.Resource.IsEmpty() {
		fmt.Fprintln(os.Stderr, "[lbox] cgroup 回读校验:")
		cg.Diagnose()
	}

	if cfg.EnableNetwork {
		if err := SetupNetwork(cmd.Process.Pid, cfg.ContainerIP); err != nil {
			return abort(fmt.Errorf("网络配置失败: %w", err))
		}
		for _, binding := range bindings {
			if err := SetupPortForward(cfg.ContainerIP, binding.hostPort, binding.containerPort); err != nil {
				return abort(fmt.Errorf("端口映射 %d:%d 失败: %w", binding.hostPort, binding.containerPort, err))
			}
			configuredPorts = append(configuredPorts, binding)
			fmt.Fprintf(os.Stderr, "[lbox] 端口映射: 宿主机 %d → 容器 %d\n", binding.hostPort, binding.containerPort)
		}
		fmt.Fprintf(os.Stderr, "[lbox] 网络已配置: 容器 IP=%s\n", cfg.ContainerIP)
		if _, err := networkWriter.Write([]byte{1}); err != nil {
			return abort(fmt.Errorf("通知容器网络就绪失败: %w", err))
		}
		if err := networkWriter.Close(); err != nil {
			return abort(fmt.Errorf("关闭网络同步管道失败: %w", err))
		}
		networkWriter = nil
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			status, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
			if ok {
				return &ExitError{Code: waitStatusExitCode(status)}
			}
			return &ExitError{Code: 1}
		}
		return fmt.Errorf("等待容器失败: %w", err)
	}
	return nil
}

func waitStatusExitCode(status syscall.WaitStatus) int {
	if status.Exited() {
		return status.ExitStatus()
	}
	if status.Signaled() {
		return 128 + int(status.Signal())
	}
	return 1
}

func printBanner(cfg *Config) {
	fmt.Fprintf(os.Stderr,
		"[lbox] 启动容器: id=%s hostname=%s namespaces=%s cmd=%v",
		cfg.ID, cfg.Hostname, cfg.Flags.String(), cfg.Command,
	)
	if !cfg.Resource.IsEmpty() {
		r := cfg.Resource
		if r.CPUCores > 0 {
			fmt.Fprintf(os.Stderr, " cpu=%.2f核", r.CPUCores)
		}
		if r.MemoryMax != "" {
			fmt.Fprintf(os.Stderr, " mem=%s", r.MemoryMax)
		}
		if r.PidsMax > 0 {
			fmt.Fprintf(os.Stderr, " pids=%d", r.PidsMax)
		}
	}
	fmt.Fprintln(os.Stderr)
}

func generateID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%08x", os.Getpid()&0xffffffff)
	}
	return hex.EncodeToString(buf[:])
}
