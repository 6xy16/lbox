#!/bin/bash
cd /mnt/d/Lunix

echo "=== 测试退出码 42 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c "exit 42"
echo "lbox exit=$?"

echo ""
echo "=== 测试正常退出码 0 ==="
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/echo "hello"
echo "lbox exit=$?"

echo ""
echo "=== 测试僵尸进程回收（容器退出后宿主机视角）==="
# 跑一个 fork 子进程然后退出的命令
./bin/lbox run --rootfs /mnt/d/Lunix/rootfs /bin/sh -c '
sleep 0.2 &
sleep 0.2 &
sleep 0.2 &
exit 0
'
echo "容器退出"
sleep 1
echo "检查宿主机有没有 zombie 残留..."
ps aux | grep -c "Z" || echo "0 个 zombie"
