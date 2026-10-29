#!/bin/bash
# 测试 lbox ps / stop / rm 完整流程
set +e
cd /mnt/d/Lunix

echo "=== 1. 清理残留 ==="
pkill -f "lbox run" 2>/dev/null
pkill -f "sleep 60" 2>/dev/null
sleep 1
# 清理所有残留 cgroup
for d in /sys/fs/cgroup/lbox/*/; do
    id=$(basename "$d")
    # 如果 cgroup.procs 为空，直接删
    if [ ! -s "$d/cgroup.procs" ]; then
        rmdir "$d" 2>/dev/null
    fi
done
./bin/lbox ps

echo ""
echo "=== 2. 启动两个容器 ==="
./bin/lbox run --name c1 /bin/sleep 60 &
sleep 1
./bin/lbox run --name c2 --cpu 0.5 --mem 128M /bin/sleep 60 &
sleep 1

echo ""
echo "=== 3. lbox ps 应该看到两个 ==="
./bin/lbox ps

echo ""
echo "=== 4. lbox stop c1 ==="
./bin/lbox stop c1
sleep 1

echo ""
echo "=== 5. lbox ps 应该只剩 c2 ==="
./bin/lbox ps

echo ""
echo "=== 6. lbox rm c1（已停止的应该能删）==="
./bin/lbox rm c1

echo ""
echo "=== 7. lbox rm c2（还在运行的应该报错）==="
./bin/lbox rm c2

echo ""
echo "=== 8. lbox stop c2 ==="
./bin/lbox stop c2
sleep 1

echo ""
echo "=== 9. lbox rm c2 ==="
./bin/lbox rm c2

echo ""
echo "=== 10. 最终状态（应该空）==="
./bin/lbox ps
