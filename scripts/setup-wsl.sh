#!/usr/bin/env bash
# =============================================================
#  lbox 开发环境一键初始化脚本 (WSL2 / Ubuntu)
#
#  用法:  bash scripts/setup-wsl.sh
#
#  作用:
#    1. 安装 Go (>=1.21)、build-essential、busybox、git、make
#    2. 校验 cgroup v2、namespace 支持
#    3. 打印环境自检报告，明确告诉你哪一步没过
#
#  注意: 必须在 WSL2 的 Ubuntu 终端里运行，不能在 Windows / Git Bash 里跑。
# =============================================================

set -euo pipefail

# ---------- 颜色输出 ----------
C_GREEN=$'\033[32m'; C_RED=$'\033[31m'; C_YELLOW=$'\033[33m'
C_BLUE=$'\033[34m';  C_BOLD=$'\033[1m';  C_OFF=$'\033[0m'

ok()   { echo "${C_GREEN}[ OK ]${C_OFF} $1"; }
warn() { echo "${C_YELLOW}[WARN]${C_OFF} $1"; }
err()  { echo "${C_RED}[FAIL]${C_OFF} $1"; }
info() { echo "${C_BLUE}[ .. ]${C_OFF} $1"; }
step() { echo; echo "${C_BOLD}==> $1${C_OFF}"; }

FAIL_COUNT=0

# =============================================================
step "0. 前置检查：确认真的在 Linux 上跑"
# =============================================================
KERNEL="$(uname -r || echo unknown)"
case "$KERNEL" in
  *microsoft*|*WSL*|*wsl*) ok "WSL 内核: $KERNEL" ;;
  *MINGW*|*MSYS*|*CYGWIN*) err "检测到 $KERNEL —— 这不是 Linux！请在 WSL2 Ubuntu 终端里运行本脚本"; exit 1 ;;
  *) ok "Linux 内核: $KERNEL（非 WSL，亦可）" ;;
esac

if [[ "$(id -u)" -eq 0 ]]; then
  warn "你正在用 root 运行，建议用普通用户 + sudo"
fi

# =============================================================
step "1. 更新软件源"
# =============================================================
info "apt update ..."
sudo apt-get update -y -qq
ok "软件源已更新"

# =============================================================
step "2. 安装基础工具 (git / make / gcc / busybox / curl)"
# =============================================================
BASE_PKGS=(git make gcc curl ca-certificates busybox uuid-runtime iproute2 iptables)
MISSING=()
for p in "${BASE_PKGS[@]}"; do
  dpkg -s "$p" >/dev/null 2>&1 || MISSING+=("$p")
done

if [[ ${#MISSING[@]} -gt 0 ]]; then
  info "安装: ${MISSING[*]}"
  sudo apt-get install -y -qq "${MISSING[@]}"
  ok "基础工具安装完成"
else
  ok "基础工具已齐全"
fi

# =============================================================
step "3. 安装 Go (>= 1.21)"
# =============================================================
GO_MIN_VER="1.23"
GO_TARGET_VER="1.23.4"   # 离线下载方案使用的版本

version_ge() { # version_ge a b  -> a >= b ?
  [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" == "$2" ]]
}

install_go_apt() {
  info "尝试通过 apt 安装 golang-go ..."
  sudo apt-get install -y -qq golang-go
}

install_go_official() {
  local arch tarball url
  arch="$(dpkg --print-architecture)"
  case "$arch" in
    amd64) arch=amd64 ;;
    arm64) arch=arm64 ;;
    *) err "不支持的架构: $arch"; return 1 ;;
  esac
  tarball="go${GO_TARGET_VER}.linux-${arch}.tar.gz"
  url="https://go.dev/dl/${tarball}"
  info "下载官方包: $url"
  curl -fsSL -o "/tmp/${tarball}" "$url"
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "/tmp/${tarball}"
  rm -f "/tmp/${tarball}"
}

if command -v go >/dev/null 2>&1; then
  CUR="$(go version | awk '{print $3}' | sed 's/^go//')"
  if version_ge "$CUR" "$GO_MIN_VER"; then
    ok "Go 已安装且满足要求: $(go version)"
  else
    warn "已装 Go $CUR 过低 (< $GO_MIN_VER)，重新安装"
    install_go_official
    ok "Go 重装完成: $(/usr/local/go/bin/go version)"
  fi
else
  install_go_apt || true
  if ! command -v go >/dev/null 2>&1 || ! version_ge "$(go version | awk '{print $3}' | sed 's/^go//')" "$GO_MIN_VER"; then
    warn "apt 源的 Go 版本过低或不可用，改用官方包"
    install_go_official
  fi
  ok "Go 安装完成"
fi

# 写入 PATH（幂等）
GO_BIN_DIR="/usr/local/go/bin"
if [[ -x "${GO_BIN_DIR}/go" ]]; then
  SHELL_RC="$HOME/.bashrc"
  grep -q 'lbox-go-path' "$SHELL_RC" 2>/dev/null || \
    cat >> "$SHELL_RC" <<'EOF'

# lbox-go-path
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
export GOPROXY=https://goproxy.cn,direct   # 国内加速
EOF
  export PATH="$PATH:${GO_BIN_DIR}:$HOME/go/bin"
  export GOPROXY=https://goproxy.cn,direct
  ok "已配置 PATH 与 GOPROXY（写入 ~/.bashrc）"
fi

# =============================================================
step "4. 校验容器运行时所需的 kernel 能力"
# =============================================================

# --- 4.1 cgroup v2 ---
CG_FS="$(stat -fc %T /sys/fs/cgroup 2>/dev/null || echo none)"
if [[ "$CG_FS" == "cgroup2fs" ]]; then
  ok "cgroup v2 已启用 (/sys/fs/cgroup = cgroup2fs)"
else
  err "cgroup v2 未启用 (当前: $CG_FS)"
  echo "     修复: 在 Windows 的 C:\\Users\\<你>\\.wslconfig 写入:"
  echo "       [wsl2]"
  echo "       kernelCommandLine = cgroup_no_v1=all systemd.unified_cgroup_hierarchy=1"
  echo "     然后 PowerShell 执行: wsl --shutdown  再重开 Ubuntu"
  FAIL_COUNT=$((FAIL_COUNT+1))
fi

# --- 4.2 CPU / 内存 / PID 控制器是否可用 ---
if [[ -f /sys/fs/cgroup/cgroup.controllers ]]; then
  CTRL="$(cat /sys/fs/cgroup/cgroup.controllers)"
  for c in cpu memory pids; do
    if grep -qw "$c" <<<"$CTRL"; then ok "控制器可用: $c"; else warn "控制器缺失: $c"; fi
  done
else
  warn "无法读取 cgroup.controllers，跳过控制器检查"
fi

# --- 4.3 namespace 支持 ---
if [[ -d /proc/self/ns ]]; then
  ok "namespace 支持正常 (/proc/self/ns 存在)"
  for ns in uts pid mnt net ipc user; do
    [[ -e "/proc/self/ns/${ns}" ]] || { warn "缺少 namespace: $ns"; }
  done
else
  err "/proc/self/ns 不存在，namespace 不可用"
  FAIL_COUNT=$((FAIL_COUNT+1))
fi

# --- 4.4 /proc 可读（监控模块依赖） ---
[[ -r /proc/meminfo ]] && ok "/proc 可读（监控模块可用）" || { err "/proc 不可读"; FAIL_COUNT=$((FAIL_COUNT+1)); }

# --- 4.5 unshare 实测：能否真的创建 namespace ---
if command -v unshare >/dev/null 2>&1; then
  if unshare --uts --pid --fork --mount-proc true >/dev/null 2>&1; then
    ok "unshare 实测通过（可创建 UTS+PID+MOUNT namespace）"
  else
    warn "unshare 实测失败，可能需要 sudo 或内核限制"
    echo "     手动验证: sudo unshare --uts --pid --fork --mount-proc /bin/sh -c 'hostname lbox; ps aux'"
  fi
fi

# =============================================================
step "5. 环境自检报告"
# =============================================================
echo
echo "${C_BOLD}--------------------------------------------${C_OFF}"
echo " 内核      : $KERNEL"
echo " Go        : $(go version 2>/dev/null || echo '未安装')"
echo " GOPROXY   : ${GOPROXY:-未设置}"
echo " cgroup    : $CG_FS"
echo " busybox   : $(command -v busybox >/dev/null && busybox --help 2>&1 | head -1 || echo '未安装')"
echo " git       : $(git --version 2>/dev/null || echo '未安装')"
echo " make      : $(make --version 2>/dev/null | head -1 || echo '未安装')"
echo "${C_BOLD}--------------------------------------------${C_OFF}"

if [[ $FAIL_COUNT -eq 0 ]]; then
  echo "${C_GREEN}${C_BOLD}环境就绪！可以开始 Phase 1 了 🚀${C_OFF}"
else
  echo "${C_RED}${C_BOLD}有 $FAIL_COUNT 项未通过，请按上面提示修复后重跑本脚本${C_OFF}"
  exit 1
fi

echo
echo "${C_BLUE}提示:${C_OFF} 让环境变量生效 ->  source ~/.bashrc"
echo "${C_BLUE}提示:${C_OFF} 建议把仓库放在 WSL 原生目录 (如 ~/lunix)，不要放 /mnt/d 下（IO 会慢很多）"
