#!/bin/bash
set +e

echo "=== WSL2 cgroup 目录结构 ==="
echo "--- /sys/fs/cgroup/ 下的目录 ---"
ls -la /sys/fs/cgroup/

echo ""
echo "--- /non-systemd cgroup 状态 ---"
cat /sys/fs/cgroup/non-systemd/cgroup.controllers
cat /sys/fs/cgroup/non-systemd/cgroup.subtree_control
echo "procs: $(cat /sys/fs/cgroup/non-systemd/cgroup.procs | wc -l) 个进程"

echo ""
echo "--- /lbox/ cgroup 状态 ---"
cat /sys/fs/cgroup/lbox/cgroup.controllers
cat /sys/fs/cgroup/lbox/cgroup.subtree_control

echo ""
echo "=== 手动测试：把当前 shell 移入 lbox 下的测试 cgroup ==="
rmdir /sys/fs/cgroup/lbox/test-move 2>/dev/null
mkdir -p /sys/fs/cgroup/lbox/test-move
echo "before: $(cat /proc/self/cgroup)"
echo $$ > /sys/fs/cgroup/lbox/test-move/cgroup.procs
echo "after:  $(cat /proc/self/cgroup)"
echo "cgroup.procs 内容: $(cat /sys/fs/cgroup/lbox/test-move/cgroup.procs)"
echo "pids.current: $(cat /sys/fs/cgroup/lbox/test-move/pids.current)"

# 把自己移回去
echo $$ > /sys/fs/cgroup/non-systemd/cgroup.procs 2>/dev/null
echo "moved back: $(cat /proc/self/cgroup)"

rmdir /sys/fs/cgroup/lbox/test-move 2>/dev/null
