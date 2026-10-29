#!/bin/bash
set +e
cd /mnt/d/Lunix

echo "=== 启动容器，同时在宿主机上追踪 ==="
./bin/lbox run --pids 8 sh -c '
echo "容器内: pid=$$, cgroup=$(cat /proc/self/cgroup)"
for i in 1 2 3 4 5 6 7 8 9 10; do
    sleep 5 &
done
sleep 3
echo "容器内: 存活 $(ps -e --no-headers | wc -l) 个进程"
' &
LB_PID=$!
sleep 2

echo ""
echo "=== 宿主机视角 ==="
echo "lbox 父进程: pid=$LB_PID, cgroup=$(cat /proc/$LB_PID/cgroup)"

# 找容器里的 sh 进程（宿主机视角）
echo ""
echo "--- 所有 sh 进程 ---"
for pid in $(pgrep -x sh); do
    echo "  sh PID $pid: cgroup=$(cat /proc/$pid/cgroup)"
done

echo ""
echo "--- 所有 sleep 进程 ---"
for pid in $(pgrep sleep); do
    echo "  sleep PID $pid: cgroup=$(cat /proc/$pid/cgroup)"
done

echo ""
echo "--- lbox 下所有 cgroup ---"
for d in /sys/fs/cgroup/lbox/*/; do
    echo "  $d"
    echo "    procs: $(cat $d/cgroup.procs 2>/dev/null | tr '\n' ' ')"
    echo "    pids.current: $(cat $d/pids.current 2>/dev/null)"
done

wait $LB_PID 2>/dev/null
