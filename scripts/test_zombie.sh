#!/bin/bash
# 测试僵尸进程回收
set +e
cd /mnt/d/Lunix

echo "=== 测试：fork 多个子进程，看有没有 zombie ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c '
echo "PID 1 = $$"
echo "fork 5 个后台进程..."
for i in 1 2 3 4 5; do
    sleep 0.1 &
done
sleep 1
echo ""
echo "当前进程列表："
ps -ef
echo ""
echo "统计 zombie 进程数："
ps -ef | grep -c "Z" || echo 0
'

echo ""
echo "=== 测试：容器退出码透传 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c 'exit 42'
echo "外层 exit=$?"
