#!/bin/bash
set +e

echo "=== 手动测 cgroup v2 pids 限制（绕开 lbox）==="

# 清理
rmdir /sys/fs/cgroup/lbox/test-pids 2>/dev/null

# 1. 创建 cgroup
mkdir -p /sys/fs/cgroup/lbox/test-pids

# 2. 在 lbox 层 subtree_control 打开 pids（如果还没开）
echo "+pids" > /sys/fs/cgroup/lbox/cgroup.subtree_control

# 3. 写 pids.max = 8
echo "8" > /sys/fs/cgroup/lbox/test-pids/pids.max
echo "pids.max = $(cat /sys/fs/cgroup/lbox/test-pids/pids.max)"

# 4. 看这个 cgroup 的 controllers
echo "本层 cgroup.controllers = $(cat /sys/fs/cgroup/lbox/test-pids/cgroup.controllers)"

# 5. 启动一个 shell，放进 cgroup
echo ""
echo "--- 启动 shell 并放入 cgroup，尝试 fork 20 个 sleep ---"
sh -c '
echo "shell pid = $$"
echo "pids.current before = $(cat /sys/fs/cgroup/lbox/test-pids/pids.current)"
for i in $(seq 1 20); do
    sleep 5 &
    echo "  fork #$i, pid=$!"
done
sleep 1
echo "pids.current after = $(cat /sys/fs/cgroup/lbox/test-pids/pids.current)"
echo "存活进程数 = $(ps -e --no-headers | wc -l)"
' &
SHELL_PID=$!

# 把这个 shell 放进 cgroup
echo $SHELL_PID > /sys/fs/cgroup/lbox/test-pids/cgroup.procs
echo "已把 shell (pid=$SHELL_PID) 写入 cgroup.procs"

wait $SHELL_PID

echo ""
echo "--- 结果 ---"
echo "pids.current final = $(cat /sys/fs/cgroup/lbox/test-pids/pids.current)"

# 清理
rmdir /sys/fs/cgroup/lbox/test-pids 2>/dev/null
