package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ContainerInfo 描述一个正在运行的容器的状态。
type ContainerInfo struct {
	ID      string
	PID     int
	CPUMax  string
	MemMax  string
	PidsMax string
}

// cgroupDirFor 校验 ID 后返回容器 cgroup 目录路径。
func cgroupDirFor(id string) (string, error) {
	if err := ValidateContainerID(id); err != nil {
		return "", err
	}
	return filepath.Join(CgroupRoot, CgroupPrefix, id), nil
}

// ListContainers 扫描 lbox cgroup 下所有正在运行的容器。
func ListContainers() ([]ContainerInfo, error) {
	parentDir := filepath.Join(CgroupRoot, CgroupPrefix)
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 cgroup 目录失败: %w", err)
	}

	var containers []ContainerInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if err := ValidateContainerID(id); err != nil {
			continue // 忽略异常目录名
		}
		cgDir := filepath.Join(parentDir, id)
		info := ContainerInfo{ID: id}
		if data, err := os.ReadFile(filepath.Join(cgDir, "cgroup.procs")); err == nil {
			firstLine := strings.TrimSpace(strings.Split(string(data), "\n")[0])
			if firstLine != "" {
				_, _ = fmt.Sscanf(firstLine, "%d", &info.PID)
			}
		}
		if info.PID <= 0 {
			// cgroup.procs 为空说明没有进程，是残留的死容器目录，不列入运行列表。
			continue
		}
		readOr := func(name string) string {
			if data, err := os.ReadFile(filepath.Join(cgDir, name)); err == nil {
				return strings.TrimSpace(string(data))
			}
			return "无限制"
		}
		info.CPUMax = readOr("cpu.max")
		info.MemMax = readOr("memory.max")
		info.PidsMax = readOr("pids.max")
		containers = append(containers, info)
	}
	return containers, nil
}

// readMainPID 读取容器 cgroup.procs 里的第一个 PID。
func readMainPID(id string) (int, error) {
	cgDir, err := cgroupDirFor(id)
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(filepath.Join(cgDir, "cgroup.procs"))
	if err != nil {
		return 0, fmt.Errorf("找不到容器 %q（可能已停止）", id)
	}
	procs := strings.Fields(string(data))
	if len(procs) == 0 {
		return 0, fmt.Errorf("容器 %q 没有运行中的进程", id)
	}
	var pid int
	_, _ = fmt.Sscanf(procs[0], "%d", &pid)
	if pid <= 0 {
		return 0, fmt.Errorf("容器 %q 的主 PID 无效", id)
	}
	return pid, nil
}

// Stop 给容器主进程组发 SIGTERM；超时未退出时再发 SIGKILL。
func Stop(id string) error {
	pid, err := readMainPID(id)
	if err != nil {
		return err
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		if err2 := syscall.Kill(pid, syscall.SIGTERM); err2 != nil {
			return fmt.Errorf("给 pid=%d 发 SIGTERM 失败: %w", err2)
		}
	}
	fmt.Printf("已发送 SIGTERM 给容器 %q (pid=%d)\n", id, pid)

	// 最多等待 10 秒，仍未退出则 SIGKILL 强制结束。
	deadline := time.Now().Add(10 * time.Second)
	for {
		if !processAlive(pid) {
			fmt.Printf("容器 %q 已停止\n", id)
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	fmt.Printf("容器 %q 超时未退出，已发送 SIGKILL\n", id)
	return nil
}

// processAlive 判断进程是否仍在运行（僵尸态视为已退出）。
func processAlive(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false // 进程已消失
	}
	// /proc/<pid>/stat 的字段用 ')' 分隔，状态字符紧随其后。
	rest := data[strings.LastIndexByte(string(data), ')')+2:]
	if len(rest) == 0 {
		return false
	}
	return rest[0] != 'Z'
}

// Remove 删除已停止容器的 cgroup 目录。仅删除目录本身，不递归删除。
func Remove(id string) error {
	cgDir, err := cgroupDirFor(id)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(cgDir, "cgroup.procs"))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("容器 %q 不存在（stop 后可能已被自动清理，或用 prune 清理残留）", id)
		}
		return err
	}
	if procs := strings.Fields(string(data)); len(procs) > 0 {
		return fmt.Errorf("容器 %q 还在运行（pid=%s），请先 stop 再 rm", id, procs[0])
	}
	if err := syscall.Rmdir(cgDir); err != nil {
		return fmt.Errorf("删除 cgroup 目录失败: %w", err)
	}
	fmt.Printf("已删除容器 %q 的 cgroup 目录\n", id)
	return nil
}

// RmAllStopped 清理所有已停止（cgroup.procs 为空）的残留容器目录。
// 独立扫描，不依赖 ListContainers（后者会过滤掉空目录）。
func RmAllStopped() (int, error) {
	parentDir := filepath.Join(CgroupRoot, CgroupPrefix)
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("读取 cgroup 目录失败: %w", err)
	}
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if err := ValidateContainerID(id); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(parentDir, id, "cgroup.procs"))
		if err != nil {
			// 目录存在但没有 cgroup.procs，也是残留，直接删。
			if syscall.Rmdir(filepath.Join(parentDir, id)) == nil {
				removed++
			}
			continue
		}
		if len(strings.Fields(string(data))) == 0 {
			if syscall.Rmdir(filepath.Join(parentDir, id)) == nil {
				removed++
			}
		}
	}
	return removed, nil
}

// execExitError 携带 exec 子进程的退出码。
type execExitError struct {
	Code int
}

func (e *execExitError) Error() string {
	return fmt.Sprintf("容器内命令退出，退出码 %d", e.Code)
}

func (e *execExitError) ExitCode() int {
	return e.Code
}

// ExecInContainer 在运行中的容器里执行一条命令。
//
// 借助 nsenter 进入容器的所有 namespace（含 mount namespace）后执行命令。
// 之所以不用纯 Go 的 setns：进入 mount namespace 要求调用线程组单线程，
// 而 Go runtime 启动即多线程，纯 Go 无法可靠 setns 进 mnt namespace（EINVAL），
// 导致 /proc 仍是宿主机挂载、ps 看到宿主机进程。nsenter 是单线程 C 程序，
// 能在单线程阶段完成全部 setns，是 docker/runc 的 exec 标准做法。
func ExecInContainer(id string, command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("用法: lbox exec <容器ID> <命令> [参数...]")
	}
	if err := ValidateContainerID(id); err != nil {
		return err
	}
	targetPID, err := readMainPID(id)
	if err != nil {
		return err
	}
	fmt.Printf("在容器 %q (pid=%d) 中执行: %v\n", id, targetPID, command)

	nsenter, err := exec.LookPath("nsenter")
	if err != nil {
		return fmt.Errorf("未找到 nsenter（util-linux），请先安装: apt install util-linux")
	}

	// --pid 会让 nsenter fork 出子进程再 exec，使命令真正处于容器 PID namespace；
	// --mount 进入容器挂载视图，使 /proc 反映容器 PID 空间。
	args := []string{
		"--target", strconv.Itoa(targetPID),
		"--mount", "--uts", "--ipc", "--net", "--pid",
		"--",
	}
	args = append(args, command...)

	cmd := exec.Command(nsenter, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err == nil {
		return nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		code := 1
		if status, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus); ok {
			code = waitStatusExitCode(status)
		}
		return &execExitError{Code: code}
	}
	return err
}
