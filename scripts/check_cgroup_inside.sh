#!/bin/bash
set +e
cd /mnt/d/Lunix

echo "=== 在 lbox 容器内检查 cgroup 归属 ==="
./bin/lbox run --pids 8 sh -c '
echo "容器内 PID 1: $$"
echo "--- 本进程的 cgroup ---"
cat /proc/self/cgroup
echo ""
echo "--- /sys/fs/cgroup 挂载情况 ---"
mount | grep cgroup
echo ""
echo "--- 尝试读 pids.current ---"
cat /sys/fs/cgroup/lbox/*/pids.current 2>/dev/null || echo "读不到"
echo ""
echo "--- 容器内 /proc 里看到的 cgroup.procs ---"
for p in /proc/[0-9]*; do
    pid=$(basename $p)
    echo "  pid=$pid cgroup=$(cat $p/cgroup 2>/dev/null)"
done
' 2>&1

echo ""
echo "=== 宿主机侧：容器退出后，看 cgroup 目录里有过哪些进程 ==="
# 容器已经退出了，cgroup 目录应该被清理了
ls /sys/fs/cgroup/lbox/ 2>&1
