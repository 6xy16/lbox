#!/bin/bash
# rootfs + cgroup 组合测试
set +e
cd /mnt/d/Lunix

echo "=== rootfs + pids 限制组合测试 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs --pids 8 /bin/sh -c '
echo hostname: $(hostname)
echo "fork 10 个 sleep:"
for i in 1 2 3 4 5 6 7 8 9 10; do
    sleep 5 &
    echo "  fork #$i, pid=$!"
done
sleep 1
echo "存活进程数: $(ps | wc -l)"
'

echo ""
echo "=== rootfs + 内存限制组合测试 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs --mem 32M /bin/sh -c '
echo 容器内开始申请内存...
# 用 dd 申请一块内存测试
dd if=/dev/zero of=/dev/null bs=1M count=100 2>&1 | tail -3
'
