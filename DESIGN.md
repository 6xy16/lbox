# lbox — 极简 Linux 容器运行时 设计文档

> 一个既能写进简历、又能撑考研复试问答的 Linux 系统编程项目。
> 定位：**200~800 行 Go 代码手写一个"迷你 Docker"**，不依赖 runc / containerd，直接调 Linux 系统调用。

---

## 一、项目一句话

> 基于 Linux **namespace** 做隔离、**cgroup v2** 做资源限制、**rootfs/pivot_root** 做文件系统视图，实现一个能跑独立 shell / 独立进程的极简容器运行时；配套一个轻量监控 agent 从 `/proc` 采集指标并以 TUI 仪表盘展示。

## 二、为什么选这个题（选型理由）

| 维度 | 评分 | 说明 |
|---|---|---|
| 简历亮点 | ★★★★★ | "手写迷你容器"在云原生/系统方向简历里非常吸睛，比"课设级 Web 系统"高一档 |
| 考研讲点 | ★★★★★ | 直接对应操作系统课的**进程管理、内存管理、文件系统**三大块，老师追问能答到底层 |
| 工作量 | ★★★★☆ | 核心 MVP 约 300 行，2~3 周可跑通；加分项按需扩展 |
| 演示性 | ★★★★☆ | `lbox run` 进入隔离环境后 `ps` 只能看到自己，效果直观 |
| 环境门槛 | ★★★☆☆ | 需要 Linux（WSL2/Ubuntu）+ root 权限，不能在 Windows 原生跑 |

**为什么不选别的：**
- 迷你 shell：太基础，本科课设都在写，区分度低。
- 迷你 Web 服务器 / Redis：网络编程经典，但"Linux 味"不够浓，老师不会觉得你懂内核。
- 容器 + 监控组合：既能讲隔离原理，又能讲 `/proc` 虚拟文件系统，**一道题能讲两层**。

---

## 三、技术栈

| 层 | 选型 | 理由 |
|---|---|---|
| 语言 | **Go 1.21+** | 云原生事实标准；`syscall` 包可直接调 `clone`/`setns`；代码量小、简历好看 |
| 隔离机制 | Linux **namespace** (7 种中的 5 种) | 容器的核心原理 |
| 资源限制 | **cgroup v2**（统一层级） | 现代发行版默认，v1 已过时 |
| 文件系统 | `pivot_root` + 普通目录 rootfs（进阶可加 **overlayfs** 分层） | 比 chroot 更安全 |
| 监控数据源 | **`/proc` 与 `/sys`** | 不依赖任何第三方库，纯读内核导出的文件 |
| TUI 展示 | [bubbletea](https://github.com/charmbracelet/bubbletea) 或手写 ANSI 转义 | 演示效果好看 |
| 构建 | Go modules + Makefile | 标准工程化 |

> 讲原理时强调：Go 只是胶水，**真正的主角是 Linux 内核提供的系统调用**，语言可替换。

---

## 四、系统架构

```
┌──────────────────────────────────────────────────────────┐
│                    lbox  CLI (用户层)                     │
│  lbox run <cmd>   lbox ps   lbox logs   lbox exec        │
└───────────────┬──────────────────────────────────────────┘
                │  命令分发
┌───────────────▼──────────────────────────────────────────┐
│                  runtime 层 (核心，自己写)                 │
│                                                          │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────────┐ │
│  │  isolation  │  │   cgroup    │  │      rootfs      │ │
│  │  namespace  │  │  cpu/mem/   │  │  pivot_root +    │ │
│  │  UTS/PID/   │  │  pid 配额   │  │  mount 隔离      │ │
│  │  MNT/NET/IPC│  │             │  │                  │ │
│  └─────────────┘  └─────────────┘  └──────────────────┘ │
│                                                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │            init 进程 (容器内 PID 1)                │  │
│  │   负责 reap 僵尸进程、设置 hostname、exec 用户命令  │  │
│  └──────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
                │
┌───────────────▼──────────────────────────────────────────┐
│                 可选加分模块：lmon agent                   │
│  读 /proc/stat /proc/meminfo /proc/loadavg /proc/net/dev │
│  → 环形缓冲采样 → TUI 仪表盘（CPU/内存/网络/进程列表）     │
└──────────────────────────────────────────────────────────┘
```

---

## 五、目录结构

```
D:\lunix\
├── README.md                  # 项目简介 + 演示动图 + 简历话术
├── DESIGN.md                  # 本文档
├── go.mod
├── Makefile
├── cmd/
│   ├── lbox/                  # CLI 主入口
│   │   └── main.go
│   └── lmon/                  # 监控 agent 入口（加分项）
│       └── main.go
├── internal/
│   ├── cli/                   # 命令解析（run/ps/exec/logs）
│   ├── runtime/              # ★ 核心：容器运行时
│   │   ├── namespace.go      #   clone flags、unshare
│   │   ├── cgroup.go         #   cgroup v2 写入
│   │   ├── rootfs.go         #   pivot_root、mount
│   │   ├── init.go            #   容器内 PID 1 逻辑
│   │   └── container.go       #   串联以上步骤
│   └── monitor/              # ★ 监控采集（加分）
│       ├── proc.go           #   /proc 解析
│       ├── ring.go           #   环形缓冲
│       └── tui.go            #   终端仪表盘
├── rootfs/                   # 一个最小 rootfs（busybox 即可）
│   └── bin/sh
└── docs/
    ├── demo.gif              # 演示录屏
    └── interview.md          # 考研/面试问答清单
```

---

## 六、核心技术点（考研讲点，老师必问）

这部分是**复试准备的弹药库**，每个点都要能讲 3~5 分钟。

### 1. Namespace：容器的"隔离墙"
- 用 `unshare(2)` / `clone(2)` 的 flag 创建：
  - `CLONE_NEWUTS` → 隔离 hostname
  - `CLONE_NEWPID` → 容器内看到自己是 PID 1
  - `CLONE_NEWNS` → 隔离 mount 表
  - `CLONE_NEWNET` → 隔离网络栈
  - `CLONE_NEWIPC` → 隔离 System V IPC
- **面试题**：为什么容器里 `ps aux` 还能看到宿主机进程？→ 因为默认没隔离 PID namespace + `/proc` 没重新挂载。
- **面试题**：容器和虚拟机的区别？→ 容器共享宿主内核，namespace 是内核级视图隔离；VM 有独立 Guest OS。

### 2. Cgroup v2：容器的"资源天花板"
- 在 `/sys/fs/cgroup/<项目名>/` 下写入：
  - `cpu.max` → CPU 配额（如 `100000 100000` = 1 核）
  - `memory.max` → 内存上限
  - `pids.max` → 最大进程数
- **面试题**：cgroup v1 和 v2 的区别？→ v1 是多层级、按资源类型分树；v2 是单层级、统一接口。
- **面试题**：容器内存满了会怎样？→ 触发 OOM killer，cgroup 内进程被回收。

### 3. Rootfs 与 pivot_root
- `chroot` 的问题：切换后老 root 还能被访问，不安全。
- `pivot_root`：把根目录真正换掉，并把旧 root 挂到 put_old 目录再 umount。
- **面试题**：容器里 `/bin/sh` 哪来的？→ 来自我们准备的 rootfs 目录（可以是 `busybox` 解压后的最小系统）。

### 4. 容器内 PID 1 的特殊性
- PID 1 不响应默认信号、**必须负责 reap 僵尸进程**，否则子进程退出后变成 zombie 堆积。
- 自己写的 init 进程要做：
  1. 设置 hostname
  2. 重新挂 `/proc`（否则看到宿主进程）
  3. `exec` 用户命令
  4. 监听 `SIGCHLD` 调用 `wait4`

### 5. /proc 虚拟文件系统（监控模块讲点）
- `/proc/stat` → CPU 时间片（user/system/idle/iowait...），两次采样差值算使用率。
- `/proc/meminfo` → 内存总量/可用/缓存。
- `/proc/loadavg` → 1/5/15 分钟负载。
- `/proc/<pid>/stat` → 单进程 CPU、状态。
- `/proc/net/dev` → 网卡收发字节数。
- **面试题**：`top` 是怎么算 CPU 使用率的？→ 读 `/proc/stat` 两次采样求差。

### 6. （进阶加分）Overlayfs 分层镜像
- lowerdir（只读层，如基础镜像）+ upperdir（可写层）+ workdir → merged。
- 讲得出"镜像分层、写时复制 CoW"，立刻拉开和普通课设的差距。

---

## 七、开发路线图（MVP → 进阶 → 亮点）

按"先跑通再优化"的顺序，每一步都能独立演示。

### Phase 0：环境准备（半天）
- [ ] WSL2 安装 Ubuntu 22.04（或虚拟机）
- [ ] 确认内核 ≥ 4.15、cgroup v2 已启用：`mount | grep cgroup2`
- [ ] 准备一个最小 rootfs：`docker export $(docker create busybox) -o rootfs.tar` 解压到 `rootfs/`

### Phase 1：MVP — 能跑一个隔离命令（3~5 天）
- [ ] `lbox run echo hello`：只用 `CLONE_NEWUTS | CLONE_NEWPID | CLONE_NEWNS`
- [ ] 容器内 `hostname` 被改成 `lbox-container`
- [ ] 容器内 `ps` 只能看到自己
- **里程碑：能用，能演示"隔离"概念。**

### Phase 2：资源限制（3~5 天）
- [ ] 实现 cgroup v2：CPU 上限、内存上限、pids 上限
- [ ] 跑一个死循环程序，验证 CPU 被压到 1 核
- [ ] 跑一个 malloc 爆内存，验证 OOM 触发
- **里程碑：简历可写"支持 CPU/内存/PID 配额"。**

### Phase 3：rootfs 与 shell（3~5 天）
- [ ] `pivot_root` 切换到 busybox rootfs
- [ ] 容器内可以跑 `/bin/sh`、`ls`、`cat`
- [ ] 正确挂 `/proc`、`/dev`
- **里程碑：`lbox run /bin/sh` 能进容器，效果接近 docker run -it。**

### Phase 4：CLI 完善（3 天）
- [ ] `lbox ps`：列出当前所有容器
- [ ] `lbox logs`：查看容器输出
- [ ] `lbox exec <id> <cmd>`：往运行中的容器里注入命令（用 `setns`）
- **里程碑：像个"产品"，不是玩具。**

### Phase 5：加分项 — 监控 agent `lmon`（按需，5~7 天）
- [ ] 读 `/proc` 采集 CPU/内存/网络/进程
- [ ] 环形缓冲保留最近 60 秒数据
- [ ] TUI 实时仪表盘（类似 btop 截图效果）
- **里程碑：项目从"运行时"升级到"运行时 + 可观测"，讲故事更完整。**

### Phase 6：文档与演示（2 天）
- [ ] 录 demo gif（进入容器 → ps 隔离 → cgroup 限制 → lmon 仪表盘）
- [ ] 写 `docs/interview.md`：把第六章的讲点整理成 Q&A
- [ ] README 放"简历话术"一段（见下）

---

## 八、简历话术（直接可抄）

> **lbox：极简 Linux 容器运行时（个人项目，Go）**
> - 基于 Linux **namespace**（UTS/PID/MNT/NET/IPC）实现进程、文件系统、网络栈隔离，不依赖 runc，直接通过 `clone(2)`/`unshare(2)` 系统调用创建容器。
> - 通过 **cgroup v2** 实现 CPU、内存、PID 数量配额，验证 OOM Killer 在 cgroup 边界内生效。
> - 使用 **`pivot_root`** 切换至 busybox 最小 rootfs，自实现容器内 PID 1，处理僵尸进程回收并重新挂载 `/proc`。
> - 配套 `lmon` agent 从 `/proc` 采集指标，环形缓冲 + TUI 实时展示主机/进程级 CPU、内存、网络数据。
> - GitHub：xxx ；Demo 录屏：xxx

---

## 九、风险与对策

| 风险 | 对策 |
|---|---|
| WSL2 默认 cgroup v2 可能没开 | `sudo grub` 加 `systemd.unified_cgroup_hierarchy=1`，或直接用 Ubuntu 22.04 |
| `pivot_root` 在某些目录报错 | 必须 `mount --make-private` 整个挂载树，否则会报 `EINVAL` |
| 没有 root 权限跑不起来 | README 明确要求 `sudo lbox run ...`，简历里说明即可 |
| Go 调 `clone` 在不同内核版本行为差异 | 锁定 kernel ≥ 4.15，README 写清楚 |
| 僵尸进程不收 | init 进程必须 `signal.Notify(SIGCHLD)` + 循环 `wait4(-1, ..., WNOHANG)` |

---

## 十、环境硬性要求

- 操作系统：**Ubuntu 22.04 / 24.04**（WSL2 或物理机均可）
- 内核：≥ 5.1（cgroup v2 成熟）
- Go：≥ 1.21
- 权限：`sudo`
- 工具：`busybox`、`git`、`make`

---

## 十·补、本机环境实测结论（2026-10-28 诊断）

对本机（Windows 11）做了实际文件系统探测，结论如下：

### 最终实测结论（第二次探测，修正第一次的误判）

第一次探测只查了 Windows 侧包目录（没找到 `ext4.vhdx`）就误判为"未初始化"，
**这个结论是错的，已作废**。第二次通过 `\\wsl.localhost\Ubuntu` 直接读取发行版文件系统，
得到真实状态：

| 检查项 | 实测结果 | 说明 |
|---|---|---|
| 内核 | ✅ **`Linux 6.18.54.1-microsoft-standard-WSL2`** | 真正的 WSL2 Linux 内核，≥5.1，cgroup v2 完备 |
| 发行版 | ✅ Ubuntu 已初始化 **且正在运行** | 根目录含 `/proc` `/sys` `/dev` `/snap` `/lost+found`；`/proc` `/sys` 时间戳为当前时刻 |
| WSL 用户名 | `xy` | 家目录 `/home/xy` |
| Go | ✅ **go1.23.4** | 装在 **`/home/xy/.local/go`**（非标准的家目录安装） |
| GOPATH / PATH | ✅ 已配好 | `.bashrc` 中 `GOPATH=/home/xy/go`，PATH 含 `.local/go/bin` 与 `$GOPATH/bin` |
| GOPROXY | ✅ `https://goproxy.cn,direct` | 国内加速已配置，拉依赖不会卡 |
| git / make / curl / tar / unshare | ✅ 均已安装 | — |
| **gcc / cc** | ❌ 缺失 | `build-essential` 未装（纯 Go + syscall 场景非必需） |
| **busybox** | ❌ 缺失 | **Phase 3 做 rootfs 时必须补装** |
| Go 模块缓存 | 空 | 还没拉过依赖 → **M1 代码刻意做成零第三方依赖** |
| Docker Desktop | ❌ 未安装 | 不影响，lbox 本就不依赖 Docker |

### 结论：环境基本就绪，只差两个小件

**内核、Go、Git、make 全都有，可以直接开工。** 唯一需要补的是：

```bash
sudo apt-get update && sudo apt-get install -y build-essential busybox-static
```

- `busybox-static` → Phase 3 构造最小 rootfs 用（静态链接，不依赖动态库，最适合塞进容器）
- `build-essential` → 备着，万一后面用到 cgo 或需要编译测试用的 C 程序（如死循环吃 CPU、malloc 爆内存来验证 cgroup）

### 待你确认的两项（我这边无法代跑）

`wsl.exe` 被本机安全策略列入黑名单，我无法从工具侧启动 WSL 执行命令，
所以以下两项只能由你在 **Ubuntu 终端**里跑，把结果贴给我：

```bash
stat -fc %T /sys/fs/cgroup          # 期望输出：cgroup2fs
cat /sys/fs/cgroup/cgroup.controllers # 期望含 cpu memory pids
go version                           # 期望：go version go1.23.4 linux/amd64
```

只要 `cgroup2fs` 和 `cpu/memory/pids` 三个控制器都在，
本项目从 M1 到 M6（含 cgroup 限制）就**全部可行，无需换环境**。

> ⚠️ 注意：本项目**不能在 Windows 原生 / Git Bash / MSYS 里开发**。
> 所有 `clone`、`pivot_root`、cgroup 操作都必须在 WSL2 的 Ubuntu 终端内进行。
> 建议把仓库放在 WSL 的原生文件系统（如 `~/lunix`）而非 `/mnt/d/lunix`，
> 否则跨文件系统 IO 会明显变慢。

---

## 下一步

确认这个方向 OK 后，我可以立刻：
1. 初始化 Go 工程骨架（go.mod / Makefile / 目录结构）
2. 写出 Phase 1 的最小可跑代码（`lbox run echo hello`）
3. 搭好 WSL2 环境检查脚本

你只要告诉我：**方向是否要调整？语言坚持 Go 还是想换成 C（更考研底层但代码量翻倍）？要不要把监控模块砍掉只做纯容器？**
