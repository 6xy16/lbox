package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// IP 池配置
const (
	ipPoolBase  = "10.0.0."
	ipPoolStart = 2
	ipPoolEnd   = 254
	ipStateDir  = "/var/lib/lbox"
	ipStateFile = "assigned_ips"
)

// AllocateIP 分配一个空闲 IP。整个过程由文件锁保护，避免并发重复分配。
func AllocateIP() (string, error) {
	release, err := lockIPState()
	if err != nil {
		return "", err
	}
	defer release()

	assigned, err := readAssignedIPsLocked()
	if err != nil {
		return "", err
	}
	set := make(map[string]struct{}, len(assigned))
	for _, ip := range assigned {
		set[ip] = struct{}{}
	}
	for i := ipPoolStart; i <= ipPoolEnd; i++ {
		ip := fmt.Sprintf("%s%d", ipPoolBase, i)
		if _, used := set[ip]; !used {
			assigned = append(assigned, ip)
			if err := writeAssignedIPsLocked(assigned); err != nil {
				return "", err
			}
			return ip, nil
		}
	}
	return "", fmt.Errorf("IP 池已满，没有可用的 IP")
}

// ReleaseIP 释放一个已分配的 IP。
func ReleaseIP(ip string) error {
	release, err := lockIPState()
	if err != nil {
		return err
	}
	defer release()

	assigned, err := readAssignedIPsLocked()
	if err != nil {
		return err
	}
	var out []string
	for _, a := range assigned {
		if a != ip {
			out = append(out, a)
		}
	}
	return writeAssignedIPsLocked(out)
}

// lockIPState 对 IP 状态文件加独占锁（flock），返回解锁函数。
func lockIPState() (func(), error) {
	if err := os.MkdirAll(ipStateDir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(ipStateDir, ipStateFile+".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开 IP 状态锁文件失败: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("锁定 IP 状态失败: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func readAssignedIPsLocked() ([]string, error) {
	path := filepath.Join(ipStateDir, ipStateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var ips []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ips = append(ips, line)
		}
	}
	return ips, nil
}

// writeAssignedIPsLocked 去重、排序后原子写入，避免并发覆盖或残留半截状态。
func writeAssignedIPsLocked(ips []string) error {
	set := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		if strings.TrimSpace(ip) != "" {
			set[ip] = struct{}{}
		}
	}
	uniq := make([]string, 0, len(set))
	for ip := range set {
		uniq = append(uniq, ip)
	}
	sort.Strings(uniq)

	tmp, err := os.CreateTemp(ipStateDir, ipStateFile+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.WriteString(strings.Join(uniq, "\n") + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(ipStateDir, ipStateFile)); err != nil {
		return err
	}
	tmpName = "" // 已成功替换
	return nil
}
