#!/bin/bash
set +e
cd /mnt/d/Lunix

echo "=== 手动对照实验：直接用 cgroup v2 文件系统测内存限制 ==="
echo ""

# 清理上次残留
rmdir /sys/fs/cgroup/lbox/test-mem 2>/dev/null

# 1. 创建测试 cgroup
mkdir -p /sys/fs/cgroup/lbox/test-mem
echo "创建目录: /sys/fs/cgroup/lbox/test-mem"

# 2. 确认 lbox 层 subtree_control 有 memory
echo "--- 写入前 lbox 层 subtree_control ---"
cat /sys/fs/cgroup/lbox/cgroup.subtree_control
echo "+memory" > /sys/fs/cgroup/lbox/cgroup.subtree_control
echo "--- 写入后 lbox 层 subtree_control ---"
cat /sys/fs/cgroup/lbox/cgroup.subtree_control

# 3. 写 memory.max = 64M
echo "--- 写入 memory.max ---"
echo "67108864" > /sys/fs/cgroup/lbox/test-mem/memory.max
cat /sys/fs/cgroup/lbox/test-mem/memory.max

# 4. 看 cgroup.type 和 controllers
echo "--- test-mem 层 cgroup.type ---"
cat /sys/fs/cgroup/lbox/test-mem/cgroup.type
echo "--- test-mem 层 cgroup.controllers ---"
cat /sys/fs/cgroup/lbox/test-mem/cgroup.controllers

# 5. 启动一个吃内存的进程，放进 cgroup
echo ""
echo "--- 启动 stress 并加入 cgroup ---"
./bin/stress --mode mem --mb 256 --secs 3 &
STRESS_PID=$!
echo "stress pid = $STRESS_PID"

# 6. 立刻把它移进 cgroup
echo $STRESS_PID > /sys/fs/cgroup/lbox/test-mem/cgroup.procs
echo "已写入 cgroup.procs"

# 7. 等待并观察
echo ""
echo "--- 等待 2 秒后观察 ---"
sleep 2
echo "memory.current = $(cat /sys/fs/cgroup/lbox/test-mem/memory.current 2>/dev/null)"
echo "memory.max = $(cat /sys/fs/cgroup/lbox/test-mem/memory.max 2>/dev/null)"
echo "cgroup.procs = $(cat /sys/fs/cgroup/lbox/test-mem/cgroup.procs 2>/dev/null)"
echo "进程还在吗? $(kill -0 $STRESS_PID 2>/dev/null && echo '活着' || echo '已退出')"

# 8. 等 stress 结束
wait $STRESS_PID 2>/dev/null
echo "stress 退出码: $?"

# 9. 看 OOM 事件
echo ""
echo "--- memory.events ---"
cat /sys/fs/cgroup/lbox/test-mem/memory.events

# 清理
rmdir /sys/fs/cgroup/lbox/test-mem 2>/dev/null
