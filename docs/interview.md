# lbox 考研复试 / 面试问答清单

> 这份文档是为"老师针对你的项目追问"准备的弹药库。
> 每个问题都按「标准答案 + 你自己项目里的实测例子」来写，背的时候把例子也一起背。

---

## 一、项目自我介绍

### 30 秒电梯版

> 我做了一个叫 lbox 的极简 Linux 容器运行时，用 Go 手写的，不依赖 Docker 和 runc。
> 核心是三件事：用 Linux namespace 做进程和文件系统隔离，用 cgroup v2 做 CPU、内存、进程数的资源限制，用 pivot_root 切换到一个 busybox 最小 rootfs。
> 跑起来效果和 `docker run` 类似：进入容器后看不到宿主机的进程、看不到宿主机的文件、资源使用被严格限制。

### 2 分钟详细版

> 这个项目的定位是"手写一个迷你 Docker"，目的是把操作系统课本里的进程管理、内存管理、文件系统三块知识落地。
>
> 技术上分三层：
> 1. **隔离层**：通过 `clone(2)` 系统调用的 flag，创建 UTS、PID、Mount、IPC 四种 namespace（启用网络时再加 Network）。UTS 隔离主机名，PID 让容器内看到自己是 PID 1，Mount 隔离挂载表，IPC 隔离 System V 信号量/共享内存。
> 2. **资源层**：cgroup v2，在 `/sys/fs/cgroup/lbox/<id>/` 下写入 `cpu.max`、`memory.max`、`pids.max` 三个文件，就能限制容器最多用多少 CPU、多少内存、多少进程。
> 3. **文件系统层**：`pivot_root` 把根目录切换到一个 busybox 做的最小 rootfs，容器里完全看不到宿主机的文件系统。
>
> 开发过程中踩过两个比较深的坑：一个是 WSL2 默认挂了 2G swap，内存撞限后内核把页换到 swap 而不是触发 OOM Killer，导致内存限制"假生效"；另一个是容器内 fork 的子进程 cgroup 归属不对，pids 限制管不住子进程。两个都通过读内核文档和逐层调试解决了。

---

## 二、Namespace 相关问答

### Q1：什么是 Linux namespace？容器和它是什么关系？

**标准答案：**
namespace 是 Linux 内核提供的**视图隔离**机制。它让一个进程（或一组进程）看到的系统资源和宿主机不一样。
目前有 7 种 namespace：UTS（主机名）、PID（进程号）、Mount（挂载点）、Network（网络栈）、IPC（进程间通信）、User（用户权限）、Cgroup（cgroup 路径视图）。
容器的本质就是：**一组进程跑在一套特殊配置的 namespace 里**。

**结合项目举例：**
我在 lbox 里用了四种（网络模式下再加 Network）：
- `CLONE_NEWUTS` → 容器内 `hostname` 是 `lbox`，改了不影响宿主机
- `CLONE_NEWPID` → 容器内 `ps` 只看到自己的进程，PID 从 1 开始
- `CLONE_NEWNS` → 容器内挂载点变化不传播回宿主机
- `CLONE_NEWIPC` → 隔离 System V IPC（信号量、共享内存、消息队列）

### Q2：为什么容器里 `ps aux` 还能看到宿主机进程？

**标准答案：**
两个原因：
1. 没创建 PID namespace，进程号空间是共享的
2. `/proc` 文件系统是宿主机视角挂载的，`ps` 读 `/proc` 自然能看到所有进程

**结合项目举例：**
我在 init 进程里专门做了一步：进入新 PID namespace 后重新 `mount -t proc proc /proc`。不做这一步的话，`ps` 会列出宿主机几百个进程，看起来就像没隔离一样。这是新手做容器最常踩的坑。

### Q3：namespace 是怎么创建的？

**标准答案：**
两个系统调用：
- `clone(2)`：fork 新进程时同时创建 namespace
- `unshare(2)`：当前进程脱离共享的 namespace，自己进入一个新的

Go 里通过 `syscall.SysProcAttr{Cloneflags: ...}` 设置，底层就是调 clone。

**结合项目举例：**
lbox 的 `run` 用"自我重执行"：父进程用 `exec.Command("/proc/self/exe", "init", ...)` 重新执行自己，同时带上 namespace flags。子进程检测到 `init` 参数，就开始做容器内的初始化（挂 /proc、改 hostname、pivot_root），最后 exec 用户命令。这和 runc 的做法是一样的。而 `exec`（进入运行中容器）则用 `nsenter`，因为 mount namespace 的 setns 要求单线程（详见 Q17 坑 4）。

### Q4：容器和虚拟机的区别？

**标准答案：**
| 维度 | 容器 | 虚拟机 |
|---|---|---|
| 内核 | 共享宿主机内核 | 有独立 Guest OS 内核 |
| 隔离级别 | 进程级隔离（namespace） | 硬件级隔离（Hypervisor） |
| 启动速度 | 毫秒级 | 秒~分钟级 |
| 资源开销 | 小（共享内核） | 大（要跑整个 OS） |
| 安全性 | 弱（共享内核，内核漏洞影响面大） | 强（硬件隔离） |

**一句话总结：** 容器是"轻量级的进程隔离"，虚拟机是"完整的硬件虚拟化"。

---

## 三、Cgroup v2 相关问答

### Q5：什么是 cgroup？它和 namespace 的区别？

**标准答案：**
- **namespace 做"隔离"**：让进程看不到外面
- **cgroup 做"限制"**：限制进程能用多少资源

一个管"看不见"，一个管"用不多"。两者结合才是完整的容器隔离。

cgroup 可以限制 CPU、内存、磁盘 IO、进程数、网络带宽等。

### Q6：cgroup v1 和 v2 有什么区别？为什么用 v2？

**标准答案：**
- **v1**：每个资源类型一棵独立的树（cpu 一棵树、memory 一棵树），层级混乱，同一个进程在不同树里可能属于不同 cgroup
- **v2**：统一层级，所有控制器在同一棵树下，接口一致，现代发行版（Ubuntu 22.04+、Debian 11+）默认启用

v2 更简洁，也是未来趋势，所以项目用的是 v2。

### Q7：cgroup v2 是怎么工作的？怎么限制 CPU 和内存？

**标准答案：**
cgroup v2 是基于**虚拟文件系统**的：
- 挂载点在 `/sys/fs/cgroup/`
- 每个 cgroup 是一个目录
- 往目录里的特定文件写数据就是配置限制

比如限制 CPU 用 0.5 核：
```bash
echo "50000 100000" > /sys/fs/cgroup/lbox/cpu.max
# 格式：quota period，100000 微秒 = 100ms，50000 就是 50ms → 0.5 核
```

限制内存 64M：
```bash
echo "67108864" > /sys/fs/cgroup/lbox/memory.max
# 64 * 1024 * 1024 = 67108864 字节
```

限制最多 8 个进程：
```bash
echo "8" > /sys/fs/cgroup/lbox/pids.max
```

把进程加入 cgroup：
```bash
echo "1234" > /sys/fs/cgroup/lbox/cgroup.procs
```

**结合项目举例：**
lbox 在 `internal/runtime/cgroup.go` 里就是做这件事：创建 `/sys/fs/cgroup/lbox/<随机ID>/` 目录，然后根据用户传的 `--cpu`、`--mem`、`--pids` 参数往对应文件里写值。

### Q8：容器内存满了会发生什么？

**标准答案：**
触发 **OOM Killer（Out-of-Memory Killer）**。
内核发现 cgroup 内的进程申请内存超过了 `memory.max`，就会挑一个"最该被杀"的进程杀掉，释放内存。
被杀的进程收到 SIGKILL 信号，无法捕获。

**结合项目举例：**
我测过：限 64M 内存，然后跑一个申请 256M 的程序，进程直接被 SIGKILL 杀掉，退出码是 -1（被信号终止）。

### Q9：（追问）你刚才说内存限制，那 swap 呢？

**标准答案：**
这个问题我实际踩过坑。
最开始我以为限了 `memory.max=64M`，超过 64M 就会 OOM。但实际测下来，进程申请 256M 没被杀，跑的好好的。

排查后发现：**WSL2 默认挂了 2G swap**。当内存撞限时，内核不是直接 OOM，而是把不活跃的匿名页换出到 swap，继续运行。

**修法：** 除了写 `memory.max`，还要写 `memory.swap.max=0`，把容器的 swap 通道堵死。这样内存撞限时就真的会触发 OOM Killer。

**这个坑的启示：** 课本里讲 OOM Killer 是"内存不够就杀进程"，但实际内核还有 swap 这一层缓冲，不把 swap 堵死，内存限制就是假的。

---

## 四、Rootfs / pivot_root 相关问答

### Q10：容器里的 `/bin/sh` 哪来的？

**标准答案：**
来自容器的 rootfs（根文件系统）。
不是宿主机的 `/bin/sh`，而是一个独立的最小文件系统镜像。
我用的是 busybox：一个静态链接的二进制文件，里面包含了几百个常用命令（sh、ls、cat、grep...），全部塞进一个 2MB 的文件里。

**结合项目举例：**
我写了个脚本 `setup-rootfs.sh`，把 busybox 复制到 `rootfs/bin/`，然后建了一堆符号链接（`ls -> busybox`、`cat -> busybox`...），再配上最小的 `/etc/passwd`、`/etc/resolv.conf`，就是一个能用的最小 rootfs 了。

### Q11：chroot 和 pivot_root 有什么区别？为什么用 pivot_root？

**标准答案：**
- **chroot**：改变进程看到的 `/`，但旧根还在，进程可以通过 `/proc/<pid>/root` 或重新 chroot 回去访问旧根。不安全。
- **pivot_root**：把根目录真正换掉，旧根被挂到一个临时目录然后 umount 掉。容器内完全看不到旧根。

Docker、runc 用的都是 pivot_root，不是 chroot。

**结合项目举例：**
我在 `rootfs.go` 里实现的标准流程：
1. `mount --bind rootfs rootfs`（确保它是一个 mount point）
2. `chdir rootfs`
3. `pivot_root(".", ".old_root")`
4. `chdir /`
5. `umount /.old_root`
6. 挂 `/proc`、`/dev`(tmpfs)、`/sys`

### Q12：为什么 pivot_root 之前要先 `mount --make-rprivate /`？

**标准答案：**
Linux 的挂载点默认是 **shared 类型**，意味着一个挂载动作会传播到所有共享同一个 peer group 的挂载点。
如果不改成 private，容器里挂 `/proc` 会反向传播到宿主机，给宿主机的 `/proc` 又叠一层挂载。

改成 private 之后，容器内的挂载变化完全不影响宿主机。

---

## 五、容器内 PID 1 相关

### Q13：为什么容器里的第一个进程（PID 1）很特殊？

**标准答案：**
PID 1 有几个特殊点：
1. **不响应默认信号**：比如 Ctrl+C 发 SIGINT 给 PID 1，默认是不生效的
2. **必须负责回收僵尸进程**：所有子进程退出后，如果 PID 1 不调用 `wait()`，就会变成 zombie 堆积
3. **是所有进程的祖先**：它死了，容器内所有进程都会被连带杀掉

**结合项目举例：**
lbox 的 init 进程就是容器内的 PID 1。它做的事：
1. 设置挂载传播为 private
2. 写 cgroup.procs（把自己加入资源限制组）
3. pivot_root 切根
4. 挂 /proc、/dev
5. 设置 hostname
6. 启动 lo 网卡（如果启用了 network namespace）
7. fork 用户命令作为子进程，父进程进入事件循环：监听 SIGCHLD 回收僵尸、转发 SIGTERM/SIGINT 信号、透传退出码

**为什么要 fork 而不是直接 exec？**
如果 PID 1 直接 exec 用户命令，那 PID 1 就变成用户命令了，僵尸回收和信号转发没人管。Docker 的做法是用 tini 做 init 进程，我们这里手写了一个简化版。

---

## 六、网络相关问答

### Q14：容器的网络是怎么实现的？

**标准答案：**
Linux 用 network namespace + veth pair + bridge + iptables 这套机制：

1. **network namespace**：每个容器有独立的网络栈（网卡、路由表、iptables）
2. **veth pair**：一对虚拟网卡，一端在容器里（eth0），一端在宿主机里，像一根网线把两个 namespace 连起来
3. **bridge**：宿主机上的一个虚拟交换机（lbox0），所有容器的 veth host 端都挂在上面
4. **iptables NAT**：把容器的源 IP 改成宿主机的 IP，这样容器就能访问外网了

**结合项目举例：**
lbox 的网络配置流程：
1. 父进程创建 bridge lbox0，配 IP 10.0.0.1/24
2. 创建 veth pair：veth-host 和 veth-cont
3. 把 veth-host 挂到 bridge 上
4. 启动容器（带 CLONE_NEWNET flag）
5. 把 veth-cont 移动到容器的 network namespace
6. 容器内 init 进程把 veth-cont 重命名为 eth0，配 IP 10.0.0.2/24，加默认路由
7. 配置 iptables MASQUERADE，让容器能访问外网

### Q15：veth pair 是什么？为什么需要一对？

**标准答案：**
veth（virtual Ethernet）是一对虚拟网卡，相当于一根虚拟网线：
- 从一端进去的数据包，会从另一端出来
- 两个端点可以在不同的 network namespace 里

为什么需要一对？因为单个网卡只能连一个 namespace，一对才能"桥接"两个 namespace。

### Q16：容器怎么访问外网？NAT 是什么原理？

**标准答案：**
容器的 IP 是 10.0.0.2（私有网段），外网不认这个地址。
所以需要 **NAT（Network Address Translation，网络地址转换）**：
- 容器发出去的包，源 IP 是 10.0.0.2
- 经过宿主机时，iptables 把源 IP 改成宿主机的公网 IP
- 外网回包时，再改回 10.0.0.2

这个动作叫 **MASQUERADE**（地址伪装）。

**结合项目举例：**
lbox 配置的规则：
```bash
iptables -t nat -A POSTROUTING -s 10.0.0.0/24 ! -d 10.0.0.0/24 -j MASQUERADE
```
意思是：所有源 IP 在 10.0.0.0/24 网段、目标不是 10.0.0.0/24 的包，都做地址伪装。

---

## 七、实际踩过的坑（老师最爱追问"你是怎么调试的"）

### Q17：做这个项目过程中遇到的最大困难是什么？

**标准答案：**
两个坑比较深：

**坑 1：内存限制假生效（swap 问题）**
- 现象：限 64M，stress 申请 256M，进程没被杀
- 排查：手动建 cgroup 对照实验，发现 `memory.current` 卡在 64M 附近，`memory.events` 里 `max=993` 但 `oom_kill=0`
- 根因：WSL2 默认挂了 2G swap，内存撞限后内核把页换到 swap，不触发 OOM
- 修法：写 `memory.swap.max=0`，堵死 swap 通道

**坑 2：pids 限制对子进程无效**
- 现象：限 8 个进程，实际 fork 出了 20 多个 sleep
- 排查：从宿主机视角追踪每个进程的 cgroup 位置，发现 sh 在 lbox cgroup 里，但它 fork 的 sleep 子进程全部跑到 `/non-systemd` cgroup 里
- 根因：父进程虽然把容器 init 写进了 cgroup.procs，但容器内 fork 的子进程 cgroup 归属不对
- 修法：让容器 init 进程在 exec 用户命令之前，自己再把自己写一遍进 cgroup.procs，确保整个进程树都在正确的 cgroup 里

**坑 3：容器 ID 路径穿越（安全问题）**
- 现象：`lbox stop ..`、`lbox rm ../x` 这类恶意 ID，能操作到 `lbox` 目录之外的 cgroup
- 根因：容器 ID 直接拼接成 cgroup 路径 `filepath.Join(CgroupRoot, "lbox", id)`，没做校验，`..` 会跳出去
- 修法：加 `ValidateContainerID`，只允许字母/数字/点/下划线/连字符，全链路（run/ps/stop/rm/exec）强制校验

**坑 4：Go 多线程导致 setns 进 mount namespace 报 EINVAL**
- 现象：`exec` 进容器后 `ps` 看到的还是宿主机进程
- 排查：`setns(fd, CLONE_NEWNS)` 返回 `invalid argument`（EINVAL），其他 namespace 都正常
- 根因：Linux 内核规定 **进入 mount namespace 要求调用线程组单线程**；而 Go runtime 启动即自带多个后台线程（GC、sysmon 等），纯 Go 进程无论如何 `LockOSThread` 都无法满足单线程前提。且 PID namespace 隔离要"可见"，必须 `/proc` 也反映容器 PID 视图，而 `/proc` 属 mount 范畴，进不去 mnt 就无解
- 修法：改用 `nsenter`（util-linux 的单线程 C 程序），在单线程阶段完成全部 setns，这是 docker/runc 的 exec 标准做法

**坑 5：Go fork 在低内存 cgroup 下 EAGAIN + Go init 被 pids 压死**
- 现象 A：`--mem 64M` 下启动用户命令报 `fork/exec: resource temporarily unavailable`
- 根因 A：容器 init 是 Go 进程，Go runtime 预留大虚拟内存；在 memory.max=64M 且 swap.max=0（禁 swap）下 fork 子进程复制页表瞬间内存撞限，内核返回 EAGAIN
- 现象 B：`--pids 4` 下容器 init 自己崩溃，报 `runtime/cgo: pthread_create failed ... SIGABRT`
- 根因 B：pids 限制同时管住了 init 进程自己的线程数，而 Go runtime 天生需要多个线程，`pthread_create` 撞上 pids.max 直接 abort
- 启示：这两个现象都源于"用 Go 写容器 init"这一设计取舍——真实 docker 的 init 是 tini 这种极轻量静态二进制，不受此影响。这是项目里最有价值的"架构反思"谈资

**调试方法论：** 遇到问题不要瞎猜，做对照实验——手动用 bash 一步步复刻流程，定位到底是哪一步出了问题。这次还有一个教训：改完代码必须重新编译，否则跑的可能是旧二进制，旧 bug 的现场会误导排查（pivot_root 的 ENOENT 就是这么被"旧二进制"耽误了一轮）。

### Q18：WSL2 环境下做容器开发有什么特殊的坑？

**标准答案：**
几个：
1. **cgroup 层级问题**：WSL2 的 systemd 会建 `/non-systemd` 这个 cgroup，很多默认进程都在里面
2. **swap 默认开启**：和物理机不一样，WSL2 默认挂了 2G swap
3. **drvfs 不支持 mknod**：项目放在 `/mnt/d/`（Windows D 盘）下时，不能创建设备节点（/dev/null 等）。解决办法是在容器运行时用 tmpfs 挂 /dev，再 mknod
4. **内核版本新**：WSL2 内核 6.18，比很多物理机新，cgroup v2 功能很全

---

## 八、和 Docker 的对比（看你对工业界理解多深）

### Q19：你的 lbox 和 Docker 差在哪里？

**标准答案：**
核心原理一样，但功能差很多：

| 功能 | lbox | Docker |
|---|---|---|
| namespace 隔离 | ✅（UTS/PID/MNT/NET） | ✅（6 种全） |
| cgroup 资源限制 | ✅（CPU/内存/pids） | ✅（CPU/内存/IO/pids/网络带宽） |
| rootfs 切换 | ✅（pivot_root + busybox） | ✅（overlayfs 分层镜像） |
| 网络 | ✅（bridge + veth + NAT） | ✅（bridge/host/overlay 多种模式） |
| 容器生命周期 | ✅（run/ps/stop/rm） | ✅（完整生命周期） |
| exec 进入运行中容器 | ✅（nsenter） | ✅（docker exec） |
| PID 1 职责 | ✅（僵尸回收 + 信号转发） | ✅（tini） |
| 监控工具 | ✅（lmon，/proc 采样） | ✅（docker stats） |
| 镜像管理 | ❌（手动准备 rootfs） | ✅（docker pull/push） |
| 分层存储 | ❌ | ✅（overlayfs + CoW） |
| 容器编排 | ❌ | ✅（docker compose / swarm） |

**一句话：** lbox 是"核心原理验证"，Docker 是"完整的工业产品"。原理上是一样的，Docker 只是加了一堆工程化的东西。

### Q20：如果让你继续做，你会加什么功能？

**标准答案：**
按优先级：
1. **overlayfs 分层**：只读层 + 可写层，实现镜像分层和写时复制
2. **动态 IP 分配**：现在固定用 10.0.0.2，同时跑两个会冲突
3. **端口映射**：把容器的端口映射到宿主机
4. **数据卷**：挂载宿主机目录到容器
5. **镜像仓库**：push/pull 镜像

---

## 九、补充：高频快问快答

| 问题 | 一句话答案 |
|---|---|
| 容器的本质是什么？ | 一组跑在特殊 namespace 里的进程 |
| namespace 和 cgroup 的区别？ | namespace 管"看不见"，cgroup 管"用不多" |
| 为什么需要重新挂 /proc？ | 进入新 PID namespace 后，旧 /proc 还是宿主机视角 |
| pivot_root 为什么比 chroot 安全？ | chroot 后旧根还能访问，pivot_root 后旧根被 umount 掉了 |
| OOM Killer 什么时候触发？ | cgroup 内内存使用超过 memory.max 且 swap 也满了 |
| 容器内 PID 1 为什么要回收僵尸进程？ | 不回收的话 zombie 会堆积，占进程号 |
| veth pair 是什么？ | 一对虚拟网卡，像网线一样连接两个 network namespace |
| 容器怎么访问外网？ | iptables NAT（MASQUERADE）把源 IP 改成宿主机 IP |
| lbox exec 原理？ | 用 nsenter(1) 进入运行中容器的 namespace（含 mount），再 fork 子进程 exec 命令 |
| 你这个项目用了多少行代码？ | 核心 runtime 大概 800 行左右（不含测试和工具） |
| 为什么用 Go 不用 C？ | Go 有垃圾回收、写起来快；而且云原生领域 Go 是事实标准。底层都是调 Linux 系统调用，语言只是胶水 |

---

## 十、复习建议

1. **先背"30 秒电梯版"自我介绍**——老师第一句话肯定是"介绍一下你的项目"
2. **重点准备"踩坑经历"**——老师最爱问"你遇到过什么困难、怎么解决的"，这是区分"抄的"和"自己做的"的关键
3. **每个技术点都要能画出来**：namespace 隔离图、cgroup 层级图、pivot_root 流程图、网络架构图（bridge + veth）
4. **准备好"现场演示"话术**：如果老师说"跑给我看看"，你能马上演示：
   - `lbox run hostname` → 容器内是 lbox
   - `lbox run --rootfs ./rootfs /bin/sh` → 进去后 `ls /` 只有 rootfs 内容
   - `lbox run --cpu 0.5 ./bin/stress --mode cpu --secs 4` → 打印"平均占用 0.50 核"，证明 CFS 限流
   - `lbox run --mem 128M ./bin/stress --mode mem --mb 256 --secs 2` → 分配到 128M 时被 OOM kill
   - `lbox run --network --rootfs ./rootfs /bin/sh` → 容器内 `ping 8.8.8.8` 能通外网
   - `lmon --once` → 实时看系统监控数据
