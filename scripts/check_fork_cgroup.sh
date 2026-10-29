#!/bin/bash
set +e
cd /mnt/d/Lunix

echo "=== 容器里 fork 子进程，看它们在哪个 cgroup ==="
./bin/lbox run --pids 8 sh -c '
echo "sh pid=$$, cgroup=$(cat /proc/self/cgroup)"
for i in 1 2 3 4 5 6 7 8 9 10; do
    sleep 5 &
    echo "fork #$i, pid=$!"
done
sleep 2
echo "--- 容器内 ps ---"
ps -e --no-headers
' &
LB_PID=$!
sleep 3

echo ""
echo "=== 宿主机上看这些 sleep 进程的 cgroup ==="
LBOX_CG=$(ls -td /sys/fs/cgroup/lbox/*/ | head -1)
echo "lbox cgroup: $LBOX_CG"
echo "cgroup.procs: $(cat $LBOX_CG/cgroup.procs)"
echo "pids.current: $(cat $LBOX_CG/pids.current)"
echo ""

for pid in $(pgrep sleep); do
    echo "sleep PID $pid: $(cat /proc/$pid/cgroup 2>/dev/null)"
done

wait $LB_PID 2>/dev/null
