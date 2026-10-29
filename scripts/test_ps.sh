#!/bin/bash
# 测试 lbox ps 命令
set +e
cd /mnt/d/Lunix

echo "=== 1. 先看空状态（应该没有运行的容器）==="
./bin/lbox ps

echo ""
echo "=== 2. 后台启动一个容器（sleep 100）==="
./bin/lbox run --name test1 --rootfs /mnt/d/Lunix/rootfs /bin/sleep 100 &
sleep 1

echo ""
echo "=== 3. 再启动一个带资源限制的容器 ==="
./bin/lbox run --name test2 --cpu 0.5 --mem 128M --pids 16 /bin/sleep 100 &
sleep 1

echo ""
echo "=== 4. 现在跑 lbox ps，应该看到两个容器 ==="
./bin/lbox ps

echo ""
echo "=== 5. 杀掉测试容器 ==="
pkill -f "lbox run" 2>/dev/null
sleep 1

echo ""
echo "=== 6. 再跑 lbox ps，应该是空的 ==="
./bin/lbox ps
