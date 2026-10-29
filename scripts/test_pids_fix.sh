#!/bin/bash
set +e
cd /mnt/d/Lunix
export GO=/home/xy/.local/go/bin/go

echo "=== 重新编译 ==="
make build GO=$GO 2>&1

echo ""
echo "=== pids 限制测试 (--pids 8) ==="
./bin/lbox run --pids 8 sh -c '
echo "容器内: pid=$$, cgroup=$(cat /proc/self/cgroup)"
for i in $(seq 1 20); do
    sleep 5 &
    echo "  fork #$i, pid=$!"
done
sleep 1
echo "  存活进程数: $(ps -e --no-headers | wc -l)"
' 2>&1

echo ""
echo "=== 宿主机侧验证 ==="
LBOX_CG=$(ls -td /sys/fs/cgroup/lbox/*/ | head -1)
echo "lbox cgroup: $LBOX_CG"
echo "cgroup.procs: $(cat $LBOX_CG/cgroup.procs | tr '\n' ' ')"
echo "pids.current: $(cat $LBOX_CG/pids.current)"
echo "pids.max: $(cat $LBOX_CG/pids.max)"
