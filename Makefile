# =============================================================
#  lbox —— 极简容器运行时
#
#  注意：必须在 WSL2 / Linux 里执行，Windows 原生（含 Git Bash）编不出
#        Linux 的 syscall 代码。
# =============================================================

# Go 可执行文件。
# 为什么要有这个变量？go 装在 ~/.local/go/bin 时依赖 .bashrc 里的 PATH，
# 而 sudo 会重置 PATH，导致 make 的 recipe 里找不到 go。
# 遇到 "make: go: No such file or directory" 时，显式指定即可：
#     make build GO=/home/xy/.local/go/bin/go
GO ?= go

BINARY := lbox
STRESS := stress
LMON := lmon
BUILD_DIR := ./bin
MAIN := ./cmd/lbox
STRESS_MAIN := ./tools/stress
LMON_MAIN := ./cmd/lmon

.PHONY: all build run demo cgroup-check fmt vet clean help

all: build

## build: 编译 lbox、验证工具 stress 与监控工具 lmon 到 ./bin/
build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY) $(MAIN)
	$(GO) build -o $(BUILD_DIR)/$(STRESS) $(STRESS_MAIN)
	$(GO) build -o $(BUILD_DIR)/$(LMON) $(LMON_MAIN)
	@echo "已构建: $(BUILD_DIR)/$(BINARY)、$(BUILD_DIR)/$(STRESS) 和 $(BUILD_DIR)/$(LMON)"

## run: 编译并进入一个隔离的 shell（需要 sudo）
run: build
	sudo $(BUILD_DIR)/$(BINARY) run /bin/sh

## demo: 一键演示全部 6 项（隔离 3 项 + cgroup 限制 3 项）
demo: build
	@echo "=========== 1) hostname 隔离 (UTS) ==========="
	@echo "宿主机 hostname: $$(hostname)"
	@echo -n "容器内 hostname: "
	@sudo $(BUILD_DIR)/$(BINARY) run hostname
	@echo
	@echo "=========== 2) 进程隔离 (PID + /proc 重挂) ==========="
	@echo "宿主机进程数: $$(ps -e --no-headers | wc -l)"
	@echo "容器内进程数: $$(sudo $(BUILD_DIR)/$(BINARY) run sh -c 'ps -e --no-headers | wc -l')"
	@echo
	@echo "=========== 3) 挂载隔离 (MOUNT): 容器内挂载不外泄 ==========="
	@echo "注意: 新建 MOUNT namespace 会【复制】宿主机挂载表，"
	@echo "      所以 mount 列表看起来一样是正常的；真正的隔离体现在"
	@echo "      容器内的挂载动作不会传播回宿主机。下面验证这一点："
	@echo "-- 容器内: 挂 tmpfs 到 /mnt 并写文件"
	@sudo $(BUILD_DIR)/$(BINARY) run sh -c 'mount -t tmpfs tmpfs /mnt && echo hello-from-container > /mnt/proof.txt && ls -A /mnt'
	@echo "-- 宿主机: /mnt 应为【空】(没有 proof.txt)，且没有 /mnt 挂载"
	@echo "   宿主机 /mnt 内容: [$$(ls -A /mnt 2>/dev/null | tr '\n' ' ')]"
	@echo "   宿主机 /mnt 挂载数: $$(mount | grep -c ' on /mnt ' || true)"
	@echo
	@echo "=========== 4) CPU 限制 (cgroup v2 cpu.max) ==========="
	@echo "-- 不限 CPU 时 (预期接近 1.00 核):"
	@sudo $(BUILD_DIR)/$(BINARY) run $(BUILD_DIR)/$(STRESS) --mode cpu --secs 4
	@echo "-- 限制 0.5 核时 (预期接近 0.50 核):"
	@sudo $(BUILD_DIR)/$(BINARY) run --cpu 0.5 $(BUILD_DIR)/$(STRESS) --mode cpu --secs 4
	@echo
	@echo "=========== 5) 内存限制 (cgroup v2 memory.max) ==========="
	@echo "-- 限 64M，实际申请 256M (预期被 OOM 杀掉，退出码非 0):"
	@sudo $(BUILD_DIR)/$(BINARY) run --mem 64M $(BUILD_DIR)/$(STRESS) --mode mem --mb 256 --secs 1 || \
		echo "   ↑ 进程被终止，符合预期（memory.max=64M 生效，触发 OOM Killer）"
	@echo
	@echo "=========== 6) PID 数量限制 (cgroup v2 pids.max) ==========="
	@echo "-- 限 8 个进程/线程，尝试一次性后台起 20 个 sleep"
	@echo "   预期: 起到第 8 个左右开始报 fork 失败(Resource temporarily unavailable)"
	@sudo $(BUILD_DIR)/$(BINARY) run --pids 8 sh -c \
		'for i in $$(seq 1 20); do sleep 5 & done; sleep 1; \
		 echo "   实际存活进程数: $$(ps -e --no-headers | wc -l)"' 2>&1 | tail -8
	@echo "   ↑ 若看到 fork 失败提示，或存活数远小于 20，说明 pids.max 生效"

## cgroup-check: 单独检查 cgroup v2 控制器状态（排查限制不生效时用）
cgroup-check:
	@echo "根  cgroup.controllers:     $$(cat /sys/fs/cgroup/cgroup.controllers 2>/dev/null)"
	@echo "根  cgroup.subtree_control: $$(cat /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null)"
	@echo "lbox 层 cgroup.controllers:     $$(cat /sys/fs/cgroup/lbox/cgroup.controllers 2>/dev/null || echo '(目录不存在)')"
	@echo "lbox 层 cgroup.subtree_control: $$(cat /sys/fs/cgroup/lbox/cgroup.subtree_control 2>/dev/null || echo '(目录不存在)')"
	@echo
	@echo "对照: 只有出现在【lbox 层 controllers】里的控制器，"
	@echo "      才能写到 /sys/fs/cgroup/lbox/<id>/ 下的对应文件。"

## fmt: 格式化代码
# 这里写成 $(GO)fmt 是因为 gofmt 和 go 在同一个 bin 目录下，
# 即 GO=go 时得到 "gofmt"，GO=/home/xy/.local/go/bin/go 时也能正确拼出路径。
fmt:
	$(GO)fmt -l -w .

## vet: 静态检查
vet:
	$(GO) vet ./...

## clean: 清理构建产物
clean:
	rm -rf $(BUILD_DIR)

## help: 显示帮助
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'
