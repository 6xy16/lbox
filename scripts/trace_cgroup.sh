#!/bin/bash
set +e
cd /mnt/d/Lunix

echo "=== 用 lbox 启动 sleep，看它在哪个 cgroup ==="
./bin/lbox run --pids 8 sleep 30 &
LB_PID=$!
sleep 2

echo "lbox 父进程 PID: $LB_PID"
echo "lbox 父进程 cgroup: $(cat /proc/$LB_PID/cgroup 2>/dev/null)"

# 找到容器里的 sleep 进程
echo ""
echo "--- 宿主机上所有 sleep 进程 ---"
ps aux | grep sleep | grep -v grep

echo ""
echo "--- 找 lbox/<id>/ 目录 ---"
ls /sys/fs/cgroup/lbox/

echo ""
echo "--- 最新的 lbox 子 cgroup ---"
LBOX_CG=$(ls -td /sys/fs/cgroup/lbox/*/ 2>/dev/null | head -1)
echo "cgroup: $LBOX_CG"
echo "cgroup.procs: $(cat $LBOX_CG/cgroup.procs 2>/dev/null)"
echo "pids.current: $(cat $LBOX_CG/pids.current 2>/dev/null)"

echo ""
echo "--- 找容器进程，看它在哪个 cgroup ---"
# 找所有在 sleep 的进程
for pid in $(pgrep -f "sleep 30"); do
    echo "PID $pid: cgroup=$(cat /proc/$pid/cgroup 2>/dev/null)"
done

# 清理
kill $LB_PID 2>/dev/null
sleep 1
