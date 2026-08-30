#!/bin/sh
set -eu

usage() {
	cat >&2 <<'EOF'
usage: build-openwrt-ipk.sh --binary PATH --package-dir PATH --version VERSION --release RELEASE --architecture ARCH --output-dir PATH
EOF
	exit 2
}

binary=
package_dir=
version=
release=
architecture=
output_dir=

while [ "$#" -gt 0 ]; do
	case "$1" in
		--binary)
			binary=$2
			shift 2
			;;
		--package-dir)
			package_dir=$2
			shift 2
			;;
		--version)
			version=$2
			shift 2
			;;
		--release)
			release=$2
			shift 2
			;;
		--architecture)
			architecture=$2
			shift 2
			;;
		--output-dir)
			output_dir=$2
			shift 2
			;;
		*)
			usage
			;;
	esac
done

[ -n "$binary" ] && [ -x "$binary" ] || usage
[ -n "$package_dir" ] && [ -d "$package_dir" ] || usage
[ -n "$version" ] && [ -n "$release" ] && [ -n "$architecture" ] && [ -n "$output_dir" ] || usage

mkdir -p "$output_dir"
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

mkdir -p \
	"$work_dir/control" \
	"$work_dir/data/usr/bin" \
	"$work_dir/data/etc/init.d" \
	"$work_dir/data/etc/config" \
	"$work_dir/data/usr/share/luci/menu.d" \
	"$work_dir/data/usr/share/rpcd/acl.d" \
	"$work_dir/data/usr/lib/lua/luci/controller" \
	"$work_dir/data/usr/lib/lua/luci/view/bagualu" \
	"$work_dir/data/usr/lib/lua/luci/model/cbi/bagualu"

install -m 0755 "$binary" "$work_dir/data/usr/bin/bagualu"
install -m 0755 "$package_dir/etc/init.d/bagualu" "$work_dir/data/etc/init.d/bagualu"
install -m 0644 "$package_dir/etc/config/bagualu" "$work_dir/data/etc/config/bagualu"
install -m 0644 "$package_dir/usr/share/luci/menu.d/luci-app-bagualu.json" "$work_dir/data/usr/share/luci/menu.d/"
install -m 0644 "$package_dir/usr/share/rpcd/acl.d/luci-app-bagualu.json" "$work_dir/data/usr/share/rpcd/acl.d/"
install -m 0644 "$package_dir/usr/lib/lua/luci/controller/bagualu.lua" "$work_dir/data/usr/lib/lua/luci/controller/"
install -m 0644 "$package_dir/usr/lib/lua/luci/view/bagualu/status.htm" "$work_dir/data/usr/lib/lua/luci/view/bagualu/"
install -m 0644 "$package_dir/usr/lib/lua/luci/view/bagualu/control.htm" "$work_dir/data/usr/lib/lua/luci/view/bagualu/"
install -m 0644 "$package_dir/usr/lib/lua/luci/model/cbi/bagualu/config.lua" "$work_dir/data/usr/lib/lua/luci/model/cbi/bagualu/"

cat > "$work_dir/control/control" <<EOF
Package: bagualu
Version: ${version}-r${release}
Architecture: ${architecture}
Maintainer: teddymail
Section: net
Priority: optional
Depends: luci-base, rpcd
Description: 八卦炉代理池
  OpenWrt 节点订阅、测速、调度和订阅输出服务。
EOF
printf '%s\n' '/etc/config/bagualu' > "$work_dir/control/conffiles"
printf '2.0\n' > "$work_dir/debian-binary"

tar -czf "$work_dir/control.tar.gz" -C "$work_dir/control" control conffiles
tar -czf "$work_dir/data.tar.gz" -C "$work_dir/data" etc usr

output="$output_dir/bagualu_${version}-r${release}_${architecture}.ipk"
python3 - "$work_dir" "$output" <<'PY'
import os
import sys

work_dir, output = sys.argv[1:]
members = ("debian-binary", "control.tar.gz", "data.tar.gz")

with open(output, "wb") as archive:
    archive.write(b"!<arch>\n")
    for name in members:
        path = os.path.join(work_dir, name)
        with open(path, "rb") as member:
            data = member.read()
        header = (
            f"{name:16.16}"
            f"{0:12d}{0:6d}{0:6d}{0o100644:8o}{len(data):10d}`\n"
        ).encode("ascii")
        archive.write(header)
        archive.write(data)
        if len(data) % 2:
            archive.write(b"\n")
PY
printf '%s\n' "$output"
