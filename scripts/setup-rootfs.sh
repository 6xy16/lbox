#!/bin/bash
# 准备 busybox 最小 rootfs
set -euo pipefail

ROOTFS_DIR="/mnt/d/Lunix/rootfs"
BUSYBOX_SRC="/usr/bin/busybox"

echo "=== 准备 rootfs: $ROOTFS_DIR ==="

# 清空旧的
rm -rf "$ROOTFS_DIR"
mkdir -p "$ROOTFS_DIR"/{bin,sbin,etc,proc,sys,dev,tmp,root,home,usr/bin,usr/sbin}

# 复制 busybox 静态二进制
cp "$BUSYBOX_SRC" "$ROOTFS_DIR/bin/busybox"
chmod +x "$ROOTFS_DIR/bin/busybox"

# 创建 busybox 符号链接（多调用二进制）
cd "$ROOTFS_DIR/bin"
for app in sh ls cat echo ps pwd mkdir rm cp mv touch grep sed awk head tail wc env hostname mount umount uname df free top kill sleep yes true false test expr basename dirname tar gzip gunzip ip route ping wget; do
    ln -sf busybox "$app"
done

# 建必要的目录（设备节点不在 rootfs 里建，容器运行时用 tmpfs 挂 /dev 动态创建）
# 原因：drvfs (/mnt/d) 不支持 mknod；即使在 ext4 上，容器内 /dev 也应该是 tmpfs

# 写一个最小 /etc/passwd
cat > "$ROOTFS_DIR/etc/passwd" <<'EOF'
root:x:0:0:root:/root:/bin/sh
nobody:x:65534:65534:nobody:/nonexistent:/bin/false
EOF

# 写一个最小 /etc/group
cat > "$ROOTFS_DIR/etc/group" <<'EOF'
root:x:0:
nogroup:x:65534:
EOF

# 写一个最小 /etc/resolv.conf（容器内 DNS 用）
cat > "$ROOTFS_DIR/etc/resolv.conf" <<'EOF'
nameserver 8.8.8.8
nameserver 114.114.114.114
EOF

echo ""
echo "=== rootfs 内容 ==="
ls -la "$ROOTFS_DIR"
echo ""
echo "=== rootfs/bin ==="
ls -la "$ROOTFS_DIR/bin" | head -20
echo ""
echo "=== 验证 busybox 可执行 ==="
chroot "$ROOTFS_DIR" /bin/busybox echo "hello from busybox rootfs"
chroot "$ROOTFS_DIR" /bin/sh -c 'echo "sh works: $(hostname)"'

echo ""
echo "✅ rootfs 准备完成: $ROOTFS_DIR"
