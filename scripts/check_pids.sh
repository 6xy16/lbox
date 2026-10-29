#!/bin/bash
set +e
cd /mnt/d/Lunix

echo "=== 手动测 pids 限制 ==="
./bin/lbox run --pids 8 sh -c '
echo "容器内进程列表:"
ps -e --no-headers
echo "--- 开始 fork 20 个 sleep ---"
for i in $(seq 1 20); do
    sleep 5 &
    echo "  fork #$i, pid=$!"
done
sleep 1
echo "--- 最终存活进程数: $(ps -e --no-headers | wc -l) ---"
ps -e --no-headers
' 2>&1

echo ""
echo "=== 直接看 cgroup 状态 ==="
LBOX_DIR=$(ls -td /sys/fs/cgroup/lbox/*/ | head -1)
echo "最新 cgroup: $LBOX_DIR"
echo "--- 父层 (lbox/) ---"
cat /sys/fs/cgroup/lbox/cgroup.controllers
cat /sys/fs/cgroup/lbox/cgroup.subtree_control
echo "--- 本层 ---"
cat $LBOX_DIR/cgroup.controllers
cat $LBOX_DIR/cgroup.subtree_control
cat $LBOX_DIR/pids.max
cat $LBOX_DIR/pids.current
cat $LBOX_DIR/cgroup.procs
