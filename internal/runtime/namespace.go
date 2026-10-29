package runtime

import (
	"strings"
)

// Linux clone(2) 的 namespace flag 常量，值与内核头文件 <linux/sched.h> 一致。
const (
	// CLONE_NEWUTS —— 隔离主机名和域名
	CloneNewUTS uintptr = 0x04000000

	// CLONE_NEWPID —— 隔离进程号空间，容器内第一个进程是 PID 1
	CloneNewPID uintptr = 0x20000000

	// CLONE_NEWNS —— 隔离挂载点表（"NS" 是历史命名，实际是 mount namespace）
	CloneNewNS uintptr = 0x00020000

	// CLONE_NEWNET —— 隔离网络栈
	CloneNewNET uintptr = 0x40000000

	// CLONE_NEWIPC —— 隔离 System V IPC 与 POSIX 消息队列
	CloneNewIPC uintptr = 0x08000000

	// CLONE_NEWUSER —— 隔离用户与组 ID（暂未启用）
	CloneNewUser uintptr = 0x10000000
)

// NamespaceFlags 描述本次容器要创建哪些命名空间。
// 本质是上面那些常量的按位或。
type NamespaceFlags uintptr

// FlagsPhase1 是当前启用的隔离组合：UTS + PID + MOUNT + IPC。
// 未启用 NEWUSER，因此需要 sudo 运行以获得创建 namespace 所需的 CAP_SYS_ADMIN。
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
