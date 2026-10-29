#!/bin/bash
set +e
cd /mnt/d/Lunix
export GO=/home/xy/.local/go/bin/go

echo "=== 完整 demo ==="
make demo GO=$GO 2>&1
echo ""
echo "=== demo 结束，退出码: $? ==="
