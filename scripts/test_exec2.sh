#!/bin/bash
cd /mnt/d/Lunix

# 清理
pkill -f "lbox run" 2>/dev/null
pkill -f "sleep 100" 2>/dev/null
sleep 1
for d in /sys/fs/cgroup/lbox/*/; do
    rmdir "$d" 2>/dev/null
done

# 启动容器
./bin/lbox run --name exec-test --rootfs /mnt/d/Lunix/rootfs /bin/sleep 100 &
sleep 2

echo "=== ps ==="
./bin/lbox ps

echo ""
echo "=== cgroup.procs ==="
cat /sys/fs/cgroup/lbox/exec-test/cgroup.procs

echo ""
echo "=== exec echo ==="
./bin/lbox exec exec-test /bin/echo "hello from exec!"
echo "exec exit=$?"

# 清理
pkill -f "lbox run" 2>/dev/null
pkill -f "sleep 100" 2>/dev/null
sleep 1
