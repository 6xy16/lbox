package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// cgroup v2 资源限制。统一层级下所有控制器挂在同一目录：
//
//	/sys/fs/cgroup/lbox/<id>/
//	├── cgroup.procs  写入 PID 即纳入限制
//	├── cpu.max       "配额 周期"，如 "50000 100000" = 0.5 核
//	├── memory.max    "256M" / "1G" / "max"
//	└── pids.max      "64"
const (
	// CgroupRoot 是 cgroup v2 的统一挂载点。
	CgroupRoot = "/sys/fs/cgroup"

	// CgroupPrefix 我们自己的 cgroup 子目录，避免和系统/其他工具冲突。
	CgroupPrefix = "lbox"

	// cpuPeriod 是 CFS 调度周期（微秒）。100000us = 100ms，是内核默认值。
	// CPU 配额 = 核数 × cpuPeriod。
	cpuPeriod = 100000

	// cgroup2SuperMagic 是 cgroup2 文件系统的魔数，用来确认真的是 v2。
	cgroup2SuperMagic = 0x63677270
)

// Resource 描述容器的资源上限。零值表示该维度不限制。
type Resource struct {
	// CPUCores：CPU 配额，单位「核」。0.5 表示最多用掉半个核。
	CPUCores float64

	// MemoryMax：内存上限，原样写入 memory.max。
	// 支持 "256M"、"1G"、"max"（不限制）。
	MemoryMax string

	// PidsMax：容器内最大进程/线程数，0 表示不限制。
	PidsMax int
}

// IsEmpty 是否完全没有设置限制（此时无需创建 cgroup）。
func (r Resource) IsEmpty() bool {
	return r.CPUCores <= 0 && r.MemoryMax == "" && r.PidsMax <= 0
}

// controllers 返回本次需要打开的控制器列表。
func (r Resource) controllers() []string {
	var out []string
	if r.CPUCores > 0 {
		out = append(out, "cpu")
	}
	if r.MemoryMax != "" {
		out = append(out, "memory")
	}
	if r.PidsMax > 0 {
		out = append(out, "pids")
	}
	return out
}

// Cgroup 表示一个 cgroup v2 控制组。
type Cgroup struct {
	ID   string
	Path string
}

// NewCgroup 生成一个 cgroup 句柄（此时还没有真正创建目录）。
func NewCgroup(id string) *Cgroup {
	return &Cgroup{
		ID:   id,
		Path: filepath.Join(CgroupRoot, CgroupPrefix, id),
	}
}

// ValidateContainerID 防止 ID 中的路径分隔符或 .. 越出 lbox cgroup 目录。
func ValidateContainerID(id string) error {
	if id == "" || len(id) > 64 {
		return fmt.Errorf("容器 ID 长度必须为 1~64 个字符")
	}
	if id == "." || id == ".." {
		return fmt.Errorf("非法容器 ID %q", id)
	}
	for _, ch := range id {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return fmt.Errorf("非法容器 ID %q：只允许字母、数字、点、下划线和连字符", id)
	}
	return nil
}

// Create 只创建 cgroup 目录，不写入任何资源限制。
// 用途：即使容器没有设 CPU/内存/pids 限制，也需要一个 cgroup 目录，
// 这样 lbox ps 才能列出所有运行中的容器。
func (c *Cgroup) Create() error {
	if err := ValidateContainerID(c.ID); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return fmt.Errorf("创建 lbox cgroup 父目录失败: %w", err)
	}
	if err := os.Mkdir(c.Path, 0o755); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("容器 ID %q 已存在，请换一个 ID 或先清理旧容器", c.ID)
		}
		return fmt.Errorf("创建 cgroup 目录失败: %w", err)
	}
	return nil
}

// Apply 创建 cgroup 目录、逐层打开控制器、写入资源上限。
// 任何一步失败都会清理已创建的目录，不留下垃圾。
func (c *Cgroup) Apply(res Resource) error {
	if res.IsEmpty() {
		return nil // 不限制就不创建，保持简单
	}

	// 0) 确认文件系统真的是 cgroup v2，而不是残留的 v1。
	if err := ensureCgroupV2(); err != nil {
		return err
	}

	// 1) 创建叶子目录（MkdirAll 会顺带创建 lbox 父目录）。
	if err := os.MkdirAll(c.Path, 0o755); err != nil {
		return fmt.Errorf("创建 cgroup 目录 %s 失败: %w", c.Path, err)
	}

	// 2) 自顶向下逐层打开控制器（v2 必须先在上层 subtree_control 里打开才能写）
	if err := c.enableControllers(res.controllers()); err != nil {
		_ = c.Remove()
		return err
	}

	// 3) 写入各项上限。
	if res.CPUCores > 0 {
		quota := int(res.CPUCores * cpuPeriod)
		if quota < 1000 {
			quota = 1000 // 内核要求 quota 最小值 1000us
		}
		val := fmt.Sprintf("%d %d", quota, cpuPeriod)
		if err := writeCgroupFile(filepath.Join(c.Path, "cpu.max"), val, "cpu"); err != nil {
			_ = c.Remove()
			return err
		}
	}
	if res.MemoryMax != "" {
		if err := writeCgroupFile(filepath.Join(c.Path, "memory.max"), res.MemoryMax, "memory"); err != nil {
			_ = c.Remove()
			return err
		}
		// 同时禁止 cgroup 使用 swap。
		// 不做这一步的坑：WSL2 / 桌面 Linux 默认挂了 swap，内存撞限后内核
		// 会把不活跃匿名页换出到 swap 而不是 OOM kill，导致"限了等于没限"
		// （进程看起来正常跑完，但 memory.current 卡在上限、swap 里堆了一堆）。
		// swap.max=0 表示这个 cgroup 完全不能用 swap，内存撞限就真的 OOM。
		// 某些旧内核可能没有 memory.swap.max 文件，写失败只警告不报错。
		if err := writeCgroupFile(filepath.Join(c.Path, "memory.swap.max"), "0", ""); err != nil {
			fmt.Fprintf(os.Stderr,
				"[lbox] 警告: 无法设置 memory.swap.max=0 (%v)。\n"+
					"       当前环境可能不支持 cgroup swap 限制；若宿主机有 swap，\n"+
					"       内存撞限后不会 OOM kill，而是换出到 swap。\n", err)
		}
	}
	if res.PidsMax > 0 {
		val := fmt.Sprintf("%d", res.PidsMax)
		if err := writeCgroupFile(filepath.Join(c.Path, "pids.max"), val, "pids"); err != nil {
			_ = c.Remove()
			return err
		}
	}
	return nil
}

// enableControllers 在本 cgroup 的所有祖先层级上打开指定控制器。
// v2 规则：子 cgroup 能用哪些控制器由父目录的 subtree_control 决定，所以要逐层打开。
func (c *Cgroup) enableControllers(ctrls []string) error {
	if len(ctrls) == 0 {
		return nil
	}

	rel, err := filepath.Rel(CgroupRoot, c.Path)
	if err != nil {
		return fmt.Errorf("计算 cgroup 相对路径失败: %w", err)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/") // 形如 ["lbox", "<id>"]

	// 先处理根，再依次处理每一级中间目录（不含叶子本身）。
	dirs := []string{CgroupRoot}
	cur := CgroupRoot
	for i := 0; i < len(parts)-1; i++ {
		cur = filepath.Join(cur, parts[i])
		dirs = append(dirs, cur)
	}

	for _, dir := range dirs {
		sub := filepath.Join(dir, "cgroup.subtree_control")
		data, err := os.ReadFile(sub)
		if err != nil {
			continue // 该层没有这个文件就跳过（例如只读的根节点）
		}

		// 只补还没打开的控制器。重复写已启用的控制器在 systemd 下会 EBUSY。
		enabled := strings.Fields(string(data))
		var missing []string
		for _, c := range ctrls {
			if !contains(enabled, c) {
				missing = append(missing, c)
			}
		}
		if len(missing) == 0 {
			continue
		}

		payload := "+" + strings.Join(missing, " +")
		if err := os.WriteFile(sub, []byte(payload), 0o644); err != nil {
			// 不直接判失败：真正的校验放在后面写 cpu.max / memory.max 时，
			// 那时报错会给出更明确的提示。这里只打个警告。
			fmt.Fprintf(os.Stderr,
				"[lbox] 警告: 在 %s 打开控制器 %v 失败(%v)。\n"+
					"       若接下来写 cpu.max / memory.max 报 ENOENT 或 EINVAL，根源就在这里。\n"+
					"       可手工确认: cat %s 与 cat %s/cgroup.controllers\n",
				dir, missing, err, sub, dir,
			)
		}
	}
	return nil
}

// contains 判断字符串切片中是否包含 s。
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// AddProcess 把一个进程纳入这个 cgroup 的限制范围。
func (c *Cgroup) AddProcess(pid int) error {
	return writeCgroupFile(
		filepath.Join(c.Path, "cgroup.procs"),
		fmt.Sprintf("%d", pid),
		"",
	)
}

// Remove 删除 cgroup 目录（等价于 rmdir）。
// 注意：目录里还有进程时会返回 EBUSY，所以必须等容器进程退出后再调用。
func (c *Cgroup) Remove() error {
	if err := syscall.Rmdir(c.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 cgroup 目录 %s 失败: %w", c.Path, err)
	}
	return nil
}

// Diagnose 回读 cgroup 关键文件并打印。v2 下写入可能成功但控制器未启用、
// 限制根本不生效且无报错，只能靠读回值对照排查。
func (c *Cgroup) Diagnose() {
	parent := filepath.Dir(c.Path)

	// 1) 父目录：有哪些控制器可用、哪些已打开
	for _, f := range []string{"cgroup.controllers", "cgroup.subtree_control"} {
		p := filepath.Join(parent, f)
		if b, err := os.ReadFile(p); err == nil {
			fmt.Fprintf(os.Stderr, "[lbox]   父层 %s = %q\n", f, strings.TrimSpace(string(b)))
		}
	}

	// 2) 本层：写入的限制值 + 当前用量 + 进程列表
	for _, f := range []string{
		"cpu.max", "memory.max", "memory.swap.max", "pids.max",
		"memory.current", "memory.swap.current", "pids.current",
		"cgroup.procs", "memory.events",
	} {
		p := filepath.Join(c.Path, f)
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[lbox]   本层 %-16s 不可读(%v) ← 该控制器未启用\n", f, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[lbox]   本层 %-16s = %q\n", f, strings.TrimSpace(string(b)))
	}
}

// ---------------------------------------------------------------------------
// 内部工具函数
// ---------------------------------------------------------------------------

// ensureCgroupV2 用 statfs 的魔数确认挂载点确实是 cgroup2。
func ensureCgroupV2() error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(CgroupRoot, &st); err != nil {
		return fmt.Errorf("无法 statfs %s: %w", CgroupRoot, err)
	}
	if int64(st.Type) != int64(cgroup2SuperMagic) {
		return fmt.Errorf(
			"%s 不是 cgroup v2（魔数=%#x）。\n"+
				"  修复: 在 Windows 的 C:\\Users\\<你>\\.wslconfig 中加入:\n"+
				"    [wsl2]\n"+
				"    kernelCommandLine = cgroup_no_v1=all systemd.unified_cgroup_hierarchy=1\n"+
				"  然后 PowerShell 执行 wsl --shutdown 再重开",
			CgroupRoot, uint64(st.Type),
		)
	}
	return nil
}

// writeCgroupFile 写一个 cgroup 控制文件，并在失败时给出人话提示。
// ctrl 用于提示"很可能是控制器没打开"，传空串则不提示。
func writeCgroupFile(path, value, ctrl string) error {
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		if ctrl != "" {
			return fmt.Errorf(
				"写入 %s = %q 失败: %w\n"+
					"  最常见原因: %q 控制器没有在上层 cgroup.subtree_control 中打开，"+
					"或该 cgroup 内已有进程导致无法改写",
				path, value, err, ctrl,
			)
		}
		return fmt.Errorf("写入 %s = %q 失败: %w", path, value, err)
	}
	return nil
}
