package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// SetupRootfs 在容器内完成根目录切换（pivot_root）。
//
// 为什么不用 chroot？
//
//	chroot 只改变进程看到的 "/",但旧 root 仍然可以通过 /proc/<pid>/root
//	或重新 chroot 回来访问,不安全。pivot_root 是把根目录真正换掉,
//	旧 root 被挂到 put_old 目录后 umount,容器内完全看不到宿主机文件系统。
//
// 标准 pivot_root 流程（参考 man 2 pivot_root 和 runc 源码）：
//  1. mount --bind rootfs 到自己（确保它是一个独立 mount point）
//  2. chdir 到 rootfs
//  3. 创建 put_old 目录（.old_root）
//  4. pivot_root(".", ".old_root")
//  5. chdir 到新根 "/"
//  6. umount 旧根 ".old_root"
//  7. 删除 put_old 目录
//  8. 挂 /proc、/dev(tmpfs)、/sys
func SetupRootfs(rootfsPath string) error {
	// 先转绝对路径：Chdir 之后，原来的相对路径不能再用来定位 rootfs。
	absRootfs, err := filepath.Abs(rootfsPath)
	if err != nil {
		return fmt.Errorf("解析 rootfs 路径失败: %w", err)
	}
	rootfsPath = absRootfs

	// 1) 确保 rootfs 目录存在
	if st, err := os.Stat(rootfsPath); err != nil {
		return fmt.Errorf("rootfs 不存在: %w", err)
	} else if !st.IsDir() {
		return fmt.Errorf("rootfs 不是目录: %s", rootfsPath)
	}

	// 2) mount --bind rootfs 到自己。
	//    为什么必须 bind？pivot_root 要求 new_root 是一个 mount point。
	//    如果 rootfs 只是一个普通目录（而不是挂载点），pivot_root 会失败。
	//    bind mount 一下就能把它变成独立的 mount point。
	if err := syscall.Mount(rootfsPath, rootfsPath, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount rootfs 失败: %w", err)
	}

	// 3) cd 到 rootfs。pivot_root 要求 new_root 是当前工作目录
	//    （或者说,内核会用 current->fs->pwd 作为 new_root）。
	if err := syscall.Chdir(rootfsPath); err != nil {
		return fmt.Errorf("chdir 到 rootfs 失败: %w", err)
	}

	// 4) 当前目录已经是 rootfs，这里只能使用新根内的相对路径。
	putOld := ".old_root"
	if err := os.Mkdir(putOld, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("创建 put_old 目录失败: %w", err)
	}

	// 5) pivot_root(".", ".old_root")
	//    第一个参数：新根（当前目录 = rootfs）
	//    第二个参数：旧根挂载点（.old_root）
	if err := syscall.PivotRoot(".", ".old_root"); err != nil {
		return fmt.Errorf("pivot_root 失败: %w", err)
	}

	// 6) cd 到新根 "/"。现在我们已经在新根里了。
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir 到新根失败: %w", err)
	}

	// 7) 旧根未成功卸载时绝不能删除目录内容，否则可能沿挂载点删到宿主机文件。
	if err := syscall.Unmount("/.old_root", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("umount 旧根失败（已保留 /.old_root，未删除任何内容）: %w", err)
	}

	// 8) 卸载成功后它应当只是空目录；只删目录本身，禁止递归删除。
	if err := os.Remove("/.old_root"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除空 put_old 目录失败: %w", err)
	}

	// 9) 挂 /proc（procfs）
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("挂 /proc 失败: %w", err)
	}

	// 10) 挂 /dev 为 tmpfs（内存文件系统,容器重启即清空）
	if err := syscall.Mount("tmpfs", "/dev", "tmpfs", 0, "mode=755"); err != nil {
		return fmt.Errorf("挂 /dev tmpfs 失败: %w", err)
	}

	// 11) 在 /dev 里创建必要的设备节点
	if err := createDevNodes(); err != nil {
		fmt.Fprintf(os.Stderr, "[lbox] 警告: 创建设备节点失败: %v\n", err)
	}

	// 12) 挂 /sys（sysfs）—— 监控模块会用到
	if err := syscall.Mount("sysfs", "/sys", "sysfs", msNosuid|msNodev|msNoexec|syscall.MS_RDONLY, ""); err != nil {
		// 不致命：WSL2 下 sysfs 可能有限制
		fmt.Fprintf(os.Stderr, "[lbox] 警告: 挂 /sys 失败: %v\n", err)
	}

	// 13) 挂 /tmp（tmpfs）
	if err := syscall.Mount("tmpfs", "/tmp", "tmpfs", 0, "mode=1777"); err != nil {
		fmt.Fprintf(os.Stderr, "[lbox] 警告: 挂 /tmp 失败: %v\n", err)
	}

	return nil
}

// createDevNodes 在 /dev 下创建容器内必要的设备节点。
// 这些是最小集合：null, zero, random, urandom, tty。
// 设备号来自 Linux 内核 Documentation/devices.txt。
func createDevNodes() error {
	devices := []struct {
		path  string
		mode  uint32
		major int
		minor int
	}{
		{"/dev/null", 0666, 1, 3},    // 字符设备 1:3
		{"/dev/zero", 0666, 1, 5},    // 字符设备 1:5
		{"/dev/full", 0666, 1, 7},    // 字符设备 1:7
		{"/dev/random", 0666, 1, 8},  // 字符设备 1:8
		{"/dev/urandom", 0666, 1, 9}, // 字符设备 1:9
		{"/dev/tty", 0666, 5, 0},     // 字符设备 5:0
	}

	for _, d := range devices {
		// mknod 系统调用：mode 里要有 S_IFCHR（字符设备）
		mode := uint32(syscall.S_IFCHR) | d.mode
		dev := int((d.major << 8) | d.minor)
		if err := syscall.Mknod(d.path, mode, dev); err != nil {
			// 已存在就跳过
			if !os.IsExist(err) {
				return fmt.Errorf("mknod %s 失败: %w", d.path, err)
			}
		}
	}

	// /dev/console 用于输出（可选）
	// 不创建：WSL2 下没有 console,创建了反而报错

	return nil
}
