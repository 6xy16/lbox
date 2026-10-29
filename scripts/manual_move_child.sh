#!/bin/bash
set +e
cd /mnt/d/Lunix

echo "=== 验证：容器内手动把进程写回 lbox cgroup ==="
./bin/lbox run --pids 8 sh -c '
CGROUP=/sys/fs/cgroup/lbox/68abb7013a553feb
echo "本进程 cgroup: $(cat /proc/self/cgroup)"
echo "lbox cgroup.procs 写入前: $(cat $CGROUP/cgroup.procs 2>/dev/null)"

for i in 1 2 3 4 5 6 7 8 9 10; do
    sleep 5 &
    child=$!
    echo $child > $CGROUP/cgroup.procs 2>/dev/null
    echo "fork #$i, pid=$child, 已移入 lbox cgroup"
done

sleep 1
echo "--- lbox cgroup pids.current: $(cat $CGROUP/pids.current) ---"
echo "--- 容器内存活: $(ps -e --no-headers | wc -l) ---"
' 2>&1

echo ""
echo "=== 宿主机侧最终状态 ==="
for d in /sys/fs/cgroup/lbox/*/; do
    echo "$d: procs=$(cat $d/cgroup.procs | tr '\n' ' ') pids.current=$(cat $d/pids.current)"
done
