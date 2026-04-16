#!/bin/bash

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "用法: $0 <listen_addr> [seconds] [output_dir]"
    echo "示例: $0 127.0.0.1:6060 20 /tmp/v2bx-pprof"
    exit 1
fi

listen_addr="$1"
seconds="${2:-20}"
output_dir="${3:-./pprof-$(date +%Y%m%d-%H%M%S)}"
base_url="http://${listen_addr}/debug/pprof"

mkdir -p "${output_dir}"

echo "collecting pprof from ${base_url}"
curl -fsS "${base_url}/goroutine?debug=1" -o "${output_dir}/goroutine.txt"
curl -fsS "${base_url}/heap" -o "${output_dir}/heap.pb.gz"
curl -fsS "${base_url}/allocs" -o "${output_dir}/allocs.pb.gz"
curl -fsS "${base_url}/profile?seconds=${seconds}" -o "${output_dir}/cpu.pb.gz"

echo "saved profiles to ${output_dir}"
