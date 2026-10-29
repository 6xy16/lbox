package runtime

import (
	"strings"
)

// ---------------------------------------------------------------------------
// Linux clone(2) 的 namespace flag 常量
//
// 这些值与内核头文件 <linux/sched.h> 中的定义一一对应。
// Go 标准库的 syscall 包虽然也导出了同名常量，但这里显式写出来有两个好处：
//  1. 教学价值：读代码的人能直接看到"容器到底向内核申请了什么"
//  2. 可控性：后续要做 flag 组合、打印诊断信息时更方便
//
// ---------------------------------------------------------------------------
const (
	// CLONE_NEWUTS  —— 隔离主机名和域名 (hostname / NIS domain name)
	// 容器内执行 hostname 看到的不再是宿主机的名字
	CloneNewUTS uintptr = 0x04000000

	// CLONE_NEWPID —— 隔离进程号空间
	// 容器内第一个进程就是 PID 1，看不到也 kill 不了宿主机的任何进程
	CloneNewPID uintptr = 0x20000000

	// CLONE_NEWNS —— 隔离挂载点表（这是 "NS" 而不是 "MOUNT" 的历史命名遗留）
	// 容器内的 mount / umount 不会影响宿主机
	CloneNewNS uintptr = 0x00020000

	// CLONE_NEWNET —— 隔离网络栈（网卡、路由表、iptables、端口）
	// 后续做 veth pair + 网桥时会用到
	CloneNewNET uintptr = 0x40000000

	// CLONE_NEWIPC —— 隔离 System V IPC 与 POSIX 消息队列
	CloneNewIPC uintptr = 0x08000000

	// CLONE_NEWUSER —— 隔离用户与组 ID
	// 让容器内以 root(uid=0) 运行，而宿主机侧只是一个普通用户
	// 注意：它必须先于其他 namespace 创建，否则没有 CAP_SYS_ADMIN 权限
	CloneNewUser uintptr = 0x10000000
)

// NamespaceFlags 描述本次容器要创建哪些命名空间。
// 本质是上面那些常量的按位或。
type NamespaceFlags uintptr

// FlagsPhase1 是 M1 里程碑使用的组合：UTS + PID + MOUNT。
// 这三个足以演示"隔离"最直观的效果：
//   - hostname 独立（UTS）
//   - ps 只能看到自己（PID + 重新挂载的 /proc）
//   - mount 操作不外泄（MOUNT）
//
// 暂时不启用 NEWUSER：它需要 uid/gid 映射（M3 才做），
// 所以 M1 阶段必须 sudo 运行以获得创建 namespace 所需的 CAP_SYS_ADMIN。
var FlagsPhase1 = NamespaceFlags(CloneNewUTS | CloneNewPID | CloneNewNS | CloneNewIPC)

// Has 判断是否启用了某个 flag。
func (f NamespaceFlags) Has(flag uintptr) bool {
	return uintptr(f)&flag != 0
}

// Uintptr 返回可直接交给 syscall.SysProcAttr.Cloneflags 的值。
func (f NamespaceFlags) Uintptr() uintptr {
	return uintptr(f)
}

// List 把启用的 namespace 翻译成人类可读的名字，用于启动时的诊断输出。
func (f NamespaceFlags) List() []string {
	var out []string
	if f.Has(CloneNewUTS) {
		out = append(out, "UTS")
	}
	if f.Has(CloneNewPID) {
		out = append(out, "PID")
	}
	if f.Has(CloneNewNS) {
		out = append(out, "MOUNT")
	}
	if f.Has(CloneNewNET) {
		out = append(out, "NET")
	}
	if f.Has(CloneNewIPC) {
		out = append(out, "IPC")
	}
	if f.Has(CloneNewUser) {
		out = append(out, "USER")
	}
	return out
}

// String 形如 "UTS+PID+MOUNT"。
func (f NamespaceFlags) String() string {
	names := f.List()
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, "+")
}
