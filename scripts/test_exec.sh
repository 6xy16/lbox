#!/bin/bash
# 测试 lbox exec
set +e
cd /mnt/d/Lunix

echo "=== 1. 先清理残留 ==="
pkill -f "lbox run" 2>/dev/null
pkill -f "sleep 100" 2>/dev/null
sleep 1
for d in /sys/fs/cgroup/lbox/*/; do
    rmdir "$d" 2>/dev/null
done

echo ""
echo "=== 2. 启动一个容器（挂 sleep 100）==="
./bin/lbox run --name exec-test --rootfs /mnt/d/Lunix/rootfs /bin/sleep 100 &
sleep 2

echo ""
echo "=== 3. lbox ps 应该看到容器在跑 ==="
./bin/lbox ps

echo ""
echo "=== 4. lbox exec 进去跑命令 ==="
./bin/lbox exec exec-test /bin/echo "hello from exec!"
echo "exec exit=$?"

echo ""
echo "=== 5. lbox exec 进去跑 hostname（应该看到 lbox）==="
./bin/lbox exec exec-test /bin/hostname

echo ""
echo "=== 6. lbox exec 进去跑 ls /（应该看到 rootfs 内容）==="
./bin/lbox exec exec-test /bin/ls /

echo ""
echo "=== 清理 ==="
./bin/lbox stop exec-test
sleep 1
./bin/lbox rm exec-test
echo "done"
