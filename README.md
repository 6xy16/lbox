# lbox —— 极简 Linux 容器运行时

> 用 Go 手写的迷你 Docker，基于 Linux namespace + cgroup v2 + pivot_root 实现。
> 不依赖 Docker / runc，从零实现容器的核心原理。

---

## 项目简介

lbox 是一个教学向的极简容器运行时，目标是把操作系统课本里的进程管理、内存管理、文件系统三块知识落地成一个能跑的项目。

核心原理：
- **Namespace** 做视图隔离（进程、文件系统、网络、主机名...）
- **Cgroup v2** 做资源限制（CPU、内存、进程数...）
- **pivot_root** 做根文件系统切换

跑起来效果和 `docker run` 类似：进入容器后看不到宿主机的进程、看不到宿主机的文件、资源使用被严格限制、有独立的网络栈。

---

## 功能特性

| 功能 | 状态 | 说明 |
|---|---|---|
| 进程隔离 | ✅ | UTS + PID + MNT + NET namespace |
| 文件系统隔离 | ✅ | pivot_root + busybox 最小 rootfs |
| CPU 限制 | ✅ | cgroup v2 cpu.max |
| 内存限制 | ✅ | cgroup v2 memory.max + swap.max=0 |
| 进程数限制 | ✅ | cgroup v2 pids.max |
| 容器生命周期 | ✅ | run / ps / stop / rm / exec |
| PID 1 职责 | ✅ | 僵尸回收 + 信号转发 + 退出码透传 |
| 网络 | ✅ | bridge + veth + NAT + 动态 IP + 端口映射 |
| 监控 | ✅ | lmon 工具，从 /proc 采样 |

---

## 快速开始

### 环境要求

- Linux（推荐 WSL2 Ubuntu 22.04+）
- Go 1.23+（`go.mod` 声明 `go 1.23`）
- root 权限（创建 namespace 和 cgroup 需要）

### 构建

```bash
git clone <repo-url>
cd lbox
make build
# 生成 ./bin/lbox、./bin/stress 和 ./bin/lmon
```

### 基本用法

```bash
# 1. 启动一个容器（最简单的例子）
sudo ./bin/lbox run /bin/sh

# 2. 启用 rootfs 隔离（容器内看不到宿主机文件）
sudo ./bin/lbox run --rootfs ./rootfs /bin/sh

# 3. 限制资源：CPU 0.5 核 + 内存 64M + 最多 8 个进程
sudo ./bin/lbox run --cpu 0.5 --mem 64M --pids 8 --rootfs ./rootfs /bin/sh

# 4. 启用网络（独立 network namespace + bridge + NAT）
sudo ./bin/lbox run --network --rootfs ./rootfs /bin/sh

# 5. 端口映射：宿主机 8080 → 容器 80
sudo ./bin/lbox run --network -p 8080:80 --rootfs ./rootfs /bin/sh

# 6. 列出运行中的容器
sudo ./bin/lbox ps

# 7. 在运行中的容器里执行命令
sudo ./bin/lbox exec <容器ID> /bin/echo "hello"

# 8. 停止容器
sudo ./bin/lbox stop <容器ID>

# 9. 删除容器
sudo ./bin/lbox rm <容器ID>
```

### 监控工具

```bash
# 实时监控系统资源（CPU/内存/网卡流量）
./bin/lmon

# 只采样一次
./bin/lmon --once
```

---

## 技术栈

| 技术 | 用途 |
|---|---|
| Go | 主体语言，调用 Linux 系统调用 |
| Linux Namespace | 进程/文件系统/网络隔离 |
| cgroup v2 | CPU/内存/进程数资源限制 |
| pivot_root | 根文件系统切换 |
| busybox | 最小 rootfs（静态链接，2MB） |
| iptables | NAT + 端口映射 |
| /proc | 监控数据采样 |

---

## 项目结构

```
lbox/
├── cmd/
│   ├── lbox/          # 主命令行入口
│   └── lmon/          # 监控 agent
├── internal/
│   └── runtime/      # 容器运行时核心
│       ├── container.go   # 主流程
│       ├── namespace.go   # namespace flags
│       ├── cgroup.go      # cgroup v2 资源限制
│       ├── rootfs.go      # pivot_root 切根
│       ├── init.go         # 容器内 PID 1
│       ├── ps.go          # ps/stop/rm/exec
│       ├── net.go         # bridge/veth/NAT/端口映射
│       └── ipam.go        # 动态 IP 分配
├── rootfs/           # busybox 最小根文件系统
├── docs/
│   └── NEXT_STEPS.md  # 后续完善方向
└── Makefile
```

---

## 简历话术

> **项目名称：lbox —— 极简 Linux 容器运行时**
>
> **项目描述：** 基于 Linux namespace + cgroup v2 从零手写的迷你容器运行时，不依赖 Docker 和 runc，实现了容器隔离、资源限制、网络、端口映射等核心功能。
>
> **技术栈：** Go、Linux Namespace、cgroup v2、pivot_root、iptables、/proc
>
> **主要工作：**
> - 用 clone(2) 系统调用创建 UTS/PID/MNT/NET 四种 namespace，实现进程、文件系统、网络的视图隔离
> - 基于 cgroup v2 虚拟文件系统实现 CPU、内存、进程数的资源限制，解决了 WSL2 swap 导致内存限制"假生效"的问题
> - 用 pivot_root 切换根文件系统，配合 busybox 构建最小 rootfs，实现容器与宿主机的文件隔离
> - 实现 bridge + veth pair + iptables NAT 的网络方案，支持动态 IP 分配和端口映射
> - 手写容器内 PID 1 进程，负责僵尸回收、信号转发和退出码透传
> - 实现 lmon 监控工具，从 /proc 文件系统采样 CPU/内存/网卡数据，做实时仪表盘

---

## License

MIT
