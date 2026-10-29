#!/bin/bash
set +e
cd /mnt/d/Lunix

echo "=== 实验：容器内 fork 子进程后，手动写回 lbox cgroup ==="

# 先启动容器，让它在后台跑
./bin/lbox run --pids 8 sh -c '
echo "容器内启动: pid=$$, cgroup=$(cat /proc/self/cgroup)"

# fork 几个 sleep，记录它们的容器内 PID
for i in 1 2 3 4 5 6 7 8 9 10; do
    sleep 5 &
    echo "fork #$i: child pid=$!"
done

# 保持容器活着
sleep 5
echo "容器退出"
' &
LB_PID=$!
sleep 2

echo ""
echo "=== 宿主机侧追踪 ==="
echo "lbox 父进程: pid=$LB_PID, cgroup=$(cat /proc/$LB_PID/cgroup)"

# 找最新的 lbox/<id>/ cgroup
LBOX_CG=$(ls -td /sys/fs/cgroup/lbox/*/ | head -1)
echo "lbox cgroup: $LBOX_CG"
echo "当前 procs: $(cat $LBOX_CG/cgroup.procs | tr '\n' ' ')"
echo "当前 pids.current: $(cat $LBOX_CG/pids.current)"

echo ""
echo "--- 所有 sleep 进程的 cgroup 归属 ---"
for pid in $(pgrep sleep); do
    echo "  sleep PID $pid: $(cat /proc/$pid/cgroup)"
done

echo ""
echo "--- 手动把所有 sleep 写进 lbox cgroup ---"
for pid in $(pgrep sleep); do
    echo $pid > $LBOX_CG/cgroup.procs 2>/dev/null && echo "  PID $pid 已写入" || echo "  PID $pid 写入失败"
done

echo ""
echo "--- 写入后 lbox cgroup 状态 ---"
echo "procs: $(cat $LBOX_CG/cgroup.procs | tr '\n' ' ')"
echo "pids.current: $(cat $LBOX_CG/pids.current)"

wait $LB_PID 2>/dev/null
