#!/bin/bash
cd /mnt/d/Lunix

echo "=== 空状态 ==="
./bin/lbox ps

echo ""
echo "=== 启动容器 test1（无资源限制）==="
./bin/lbox run --name test1 /bin/sleep 30 &
sleep 2

echo ""
echo "=== 启动容器 test2（带资源限制）==="
./bin/lbox run --name test2 --cpu 0.5 --mem 128M --pids 16 /bin/sleep 30 &
sleep 2

echo ""
echo "=== lbox ps 输出 ==="
./bin/lbox ps

echo ""
echo "=== 清理 ==="
pkill -f "lbox run" 2>/dev/null
pkill -f "sleep 30" 2>/dev/null
sleep 2

echo ""
echo "=== 清理后 ==="
./bin/lbox ps
