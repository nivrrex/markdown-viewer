#!/bin/sh
# 把 mygo v0.10.0 复制成本地 fork 并打上 patches/ 里的补丁。
#
# 为什么需要这一步：mygo v0.10.0 在 Windows 上对字形启用 ClearType 子像素覆盖，
# 中文小字号会出现红蓝彩边，观感是颗粒感。上游尚未修复，因此保留一个只含该修复的
# 本地 fork，由 go.mod 的 replace 指向它。
#
# 用法：
#   sh scripts/setup-mygo-fork.sh
#   go tool mygo build -platform windows/amd64 -skip-installer
#
# 环境变量：
#   GO              Go 可执行文件，默认 go
#   MYGO_VERSION   要 fork 的 mygo 版本，默认 v0.10.0
#   MYGO_FORK_DIR  fork 的落点，默认 .mygo-fork/mygo；相对路径按项目根目录解析
set -eu

version="${MYGO_VERSION:-v0.10.0}"
go_bin="${GO:-go}"
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
dir="${MYGO_FORK_DIR:-$root/.mygo-fork/mygo}"
case "$dir" in
/*) ;;
*) dir="$root/$dir" ;;
esac
patchfile="$root/patches/mygo-cleartype-grayscale.patch"
tmp=""
created=0

cleanup() {
	if [ -n "$tmp" ]; then
		rm -rf -- "$tmp"
	fi
	if [ "$created" -eq 1 ] && [ -d "$dir" ]; then
		rm -rf -- "$dir"
	fi
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

if ! command -v "$go_bin" >/dev/null 2>&1; then
	echo "找不到 Go 可执行文件：$go_bin" >&2
	exit 1
fi
if ! (cd "$root" && GOTOOLCHAIN=local "$go_bin" list -m -f '{{.GoVersion}}' >/dev/null); then
	echo "当前 Go 版本低于 go.mod 要求，或 go.mod 无法解析；不会自动下载工具链" >&2
	exit 1
fi
if [ ! -f "$patchfile" ]; then
	echo "找不到 $patchfile" >&2
	exit 1
fi

patch_applied() {
	patch -R -p1 --dry-run --force -d "$dir" <"$patchfile" >/dev/null 2>&1
}

if [ -d "$dir" ]; then
	if patch_applied; then
		echo "$dir 已存在，补丁已应用" >&2
		exit 0
	fi
	if ! patch -p1 --forward --dry-run -d "$dir" <"$patchfile" >/dev/null 2>&1; then
		echo "$dir 已存在，但补丁状态无法确认；请检查或移走该目录后重试" >&2
		exit 1
	fi
else
	modcache=$(GOTOOLCHAIN=local "$go_bin" env GOMODCACHE)
	src="$modcache/github.com/egoist/mygo@$version"
	if [ ! -d "$src" ]; then
		echo "模块缓存里没有 mygo $version，正在下载" >&2
		tmp=$(mktemp -d)
		(cd "$tmp" && GOTOOLCHAIN=local "$go_bin" mod init mygo-fork-setup >/dev/null 2>&1 &&
			GOTOOLCHAIN=local "$go_bin" mod download "github.com/egoist/mygo@$version")
	fi
	if [ ! -d "$src" ]; then
		echo "下载后仍找不到 $src" >&2
		exit 1
	fi

	echo "从 $src 复制到 $dir" >&2
	mkdir -p "$(dirname "$dir")"
	created=1
	cp -a "$src" "$dir"
	chmod -R u+w "$dir"
fi

echo "应用 $patchfile" >&2
if ! (cd "$dir" && patch -p1 --forward <"$patchfile"); then
	echo "补丁应用失败" >&2
	exit 1
fi

created=0
echo "完成：$dir" >&2
