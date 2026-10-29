# lbox 项目 —— 你需要做什么 & 完善路线

## 一、现在立刻要做（一条命令，1 分钟）

我这边无法访问 `go` 和 WSL，所以代码只做了静态修复，**还没实机编译**。请在 WSL 的 Ubuntu 终端里跑：

```bash
cd /mnt/d/lunix && gofmt -w cmd internal tools && go build ./... && go vet ./... && go test ./...
```

- 有报错：把完整报错贴给我，我来改。
- 全绿：继续往下。

> 注意：仓库在 `/mnt/d/lunix`（Windows 盘）。DrvFS 上 IO 慢、`mknod` 受限，rootfs 和构建建议放 WSL 原生目录，例如 `cp -r /mnt/d/lunix ~/lunix` 后在 `~/lunix` 里跑。但 DrvFS 上**编译运行本身没问题**，先跑通再说。

## 二、跑一遍冒烟测试（验证修复是否真的生效）

按顺序执行，每步看输出是否符合预期：

```bash
cd /mnt/d/lunix && make build

# 1. 基本隔离（UTS + PID + IPC + MOUNT）
sudo ./bin/lbox run /bin/sh -c 'echo hostname=$(hostname); ps -e | wc -l'

# 2. rootfs 切根（重点验证 pivot_root 相对路径修复）
sudo ./bin/lbox run --rootfs ./rootfs /bin/sh -c 'ls /; hostname'

# 3. 自定义主机名（验证 --hostname 修复）
sudo ./bin/lbox run --hostname mybox /bin/sh -c 'hostname'

# 4. 资源限制（cgroup v2）
sudo ./bin/lbox run --cpu 0.5 --mem 64M --pids 8 ./bin/stress --mode cpu --secs 4

# 5. 网络 + 端口映射（需要 iproute2/iptables）
sudo ./bin/lbox run --network --rootfs ./rootfs /bin/sh

# 6. 生命周期
sudo ./bin/lbox run --name demo --rootfs ./rootfs /bin/sleep 100 &   # 后台
sudo ./bin/lbox ps
sudo ./bin/lbox exec demo /bin/echo hello   # 验证 exec 进 PID namespace 后 ps 只看到容器
sudo ./bin/lbox stop demo                    # 验证超时强杀
sudo ./bin/lbox rm demo
```

关键验证点：
- 步骤 2 不再报 `pivot_root` 失败，也不残留 `rootfs/rootfs/.old_root`。
- 步骤 3 输出 `mybox` 而不是 `lbox`。
- 步骤 6 的 `exec ... /bin/ps` 在 PID namespace 隔离下应该只看到容器内进程（若容器是 `/bin/sleep`，则看到 sleep + 少量）。
- 停止容器后 `ls /sys/fs/cgroup/lbox/` 不再残留目录。

## 三、更进一步完善项目的路线（按优先级）

### P0 —— 正确性收尾（现在这批改动的验证）
- 跑完上面两条命令，把结果回给我。

### P1 —— 隔离性（面试最加分）
1. **user namespace + rootless（最重要）**：当前容器内 root 仍是宿主机 root，只能跑可信命令。加 `CLONE_NEWUSER` + `/proc/self/uid_map`/`gid_map` + capability 降权后，才能 `sudo` 都不需要、真正隔离 uid。这是「容器 vs 虚拟机」面试题的硬核答案。
2. **overlayfs 分层 rootfs**：把只读 base 层 + 可写 upper 层叠起来，讲「镜像分层 / 写时复制 CoW」，直接和普通课设拉开差距（DESIGN.md 已留了接口）。
3. **seccomp + capabilities**：限制容器可用系统调用和特权，讲「最小权限」。

### P2 —— 工程化
4. **`lbox logs`**：把容器 stdout/stderr 落盘（或管道），实现日志查询。
5. **`lbox run --rm`**：容器退出自动清理，不用手动 `rm`。
6. **TTY 支持**：`--tty` 分配伪终端，让交互式 shell 有提示符、支持 Ctrl+C（当前 `Config.Tty` 是空的）。
7. **优雅退出**：SIGTERM 等待窗口 + 超时 SIGKILL 已加，可进一步加「SIGKILL 后确认」。
8. **cgroup 资源更全**：`io.max`、`cpuset`、`memory.high`（软限制）。

### P3 —— 可观测性 & 测试
9. **`lmon` 增强**：进程列表、每个容器的 CPU/内存（按 cgroup 读 `cpu.stat`/`memory.current`）、负载、环形缓冲图表。
10. **测试补全**：给 `parsePortBinding`、`ValidateContainerID`、IPAM（用临时目录）补表驱动测试；加一个集成脚本一键验证全部功能。
11. **CI**：GitHub Actions 跑 `gofmt/vet/test`（纯 Linux runner，避免只在 WSL 里能编的盲区）。

### P4 —— 简历/文档
12. **demo 录屏**：`run → ps 隔离 → cgroup 限制 → lmon 仪表盘` 一段 GIF，放 README。
13. **Git 仓库**：当前不是 git 仓库，先 `git init` 提交，把 DESIGN.md 里「GitHub: xxx」补上真实链接。
14. **interview.md 更新**：把「exec 进 PID namespace」「IPC 隔离」「stop 超时强杀」补进问答清单。

## 四、常见坑速查

| 症状 | 原因 | 处理 |
|---|---|---|
| `make: go: No such file or directory` | sudo 重置 PATH | `make build GO=/home/xy/.local/go/bin/go` |
| `cpu.max` 写失败 ENOENT/EINVAL | cgroup 控制器没开 | 看 `make cgroup-check`，`cgroup.subtree_control` 补 `+cpu +memory +pids` |
| pivot_root EINVAL | 挂载树没设 private | 代码已做 `make --rprivate /`，仍报错检查是否用 WSL 原生目录 |
| 内存限了但没 OOM | swap 没禁 | 代码已写 `memory.swap.max=0`，确认 `/sys/fs/cgroup/lbox/<id>/memory.swap.max` 为 0 |
| 网络配不上 | 缺 `iproute2`/`iptables` | `sudo apt install iproute2 iptables` |
