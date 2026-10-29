#!/bin/bash
set +e

echo "=== 确认 swap 是否被使用 ==="

# 清理
rmdir /sys/fs/cgroup/lbox/test-mem 2>/dev/null
mkdir -p /sys/fs/cgroup/lbox/test-mem
echo "+memory" > /sys/fs/cgroup/lbox/cgroup.subtree_control 2>/dev/null
echo "67108864" > /sys/fs/cgroup/lbox/test-mem/memory.max

# 先看全局 swap
echo "--- 全局 swap 状态 ---"
swapon --show
free -h

echo ""
echo "--- 跑 stress，后台采样 cgroup ---"
./bin/stress --mode mem --mb 256 --secs 3 &
SPID=$!
echo $SPID > /sys/fs/cgroup/lbox/test-mem/cgroup.procs

# 每秒采样一次
for i in 1 2 3; do
    sleep 1
    CUR=$(cat /sys/fs/cgroup/lbox/test-mem/memory.current 2>/dev/null)
    SWAP=$(cat /sys/fs/cgroup/lbox/test-mem/memory.swap.current 2>/dev/null)
    SWAPMAX=$(cat /sys/fs/cgroup/lbox/test-mem/memory.swap.max 2>/dev/null)
    echo "  [采样$i] memory.current=$CUR  memory.swap.current=$SWAP  swap.max=$SWAPMAX"
done

wait $SPID 2>/dev/null
echo "stress 退出码: $?"

echo ""
echo "--- 再试一次: 禁止 swap (memory.swap.max=0) ---"
rmdir /sys/fs/cgroup/lbox/test-mem 2>/dev/null
mkdir -p /sys/fs/cgroup/lbox/test-mem
echo "67108864" > /sys/fs/cgroup/lbox/test-mem/memory.max
echo "0" > /sys/fs/cgroup/lbox/test-mem/memory.swap.max
echo "memory.swap.max = $(cat /sys/fs/cgroup/lbox/test-mem/memory.swap.max)"

./bin/stress --mode mem --mb 256 --secs 2 &
SPID=$!
echo $SPID > /sys/fs/cgroup/lbox/test-mem/cgroup.procs
wait $SPID 2>/dev/null
echo "stress 退出码: $?"

echo ""
echo "--- memory.events ---"
cat /sys/fs/cgroup/lbox/test-mem/memory.events

rmdir /sys/fs/cgroup/lbox/test-mem 2>/dev/null
