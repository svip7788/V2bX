#!/bin/sh
if [ "$(id -u)" -ne 0 ]; then SUDO="sudo"; else SUDO=""; fi

echo ">>> 开启 BBR..."
$SUDO modprobe tcp_bbr 2>/dev/null || true
if ! grep -q "tcp_bbr" /etc/modules-load.d/modules.conf 2>/dev/null; then
    echo "tcp_bbr" | $SUDO tee -a /etc/modules-load.d/modules.conf >/dev/null 2>/dev/null || true
fi

$SUDO tee /etc/sysctl.d/99-bbr-optimize.conf > /dev/null << 'EOF'
net.core.default_qdisc = fq
net.ipv4.tcp_congestion_control = bbr
net.ipv4.tcp_fastopen = 3
net.ipv4.tcp_slow_start_after_idle = 0
net.ipv4.tcp_no_metrics_save = 1
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_keepalive_probes = 5
net.ipv4.tcp_keepalive_intvl = 15
net.ipv4.tcp_max_tw_buckets = 2000000
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_mtu_probing = 1
net.ipv4.tcp_syncookies = 1
net.ipv4.tcp_max_syn_backlog = 65536
net.ipv4.tcp_synack_retries = 2
net.ipv4.tcp_syn_retries = 2
net.core.rmem_max = 67108864
net.core.wmem_max = 67108864
net.core.rmem_default = 1048576
net.core.wmem_default = 1048576
net.core.netdev_max_backlog = 65536
net.core.somaxconn = 65536
net.ipv4.tcp_rmem = 4096 1048576 67108864
net.ipv4.tcp_wmem = 4096 1048576 67108864
net.ipv4.udp_rmem_min = 8192
net.ipv4.udp_wmem_min = 8192
net.ipv4.tcp_mem = 786432 1048576 26777216
net.ipv4.tcp_window_scaling = 1
net.ipv4.tcp_adv_win_scale = -2
net.ipv4.tcp_notsent_lowat = 131072
#net.netfilter.nf_conntrack_max = 2000000
#net.netfilter.nf_conntrack_tcp_timeout_established = 7200
#net.netfilter.nf_conntrack_tcp_timeout_close_wait = 60
#net.netfilter.nf_conntrack_tcp_timeout_fin_wait = 60
#net.netfilter.nf_conntrack_tcp_timeout_time_wait = 60
net.ipv6.conf.all.disable_ipv6 = 0
net.ipv6.conf.default.disable_ipv6 = 0
fs.file-max = 6815744
fs.nr_open = 6815744
EOF

$SUDO sysctl --system > /dev/null 2>&1 || true

if ! grep -q "65535" /etc/security/limits.conf 2>/dev/null; then
    printf '* soft nofile 655350\n* hard nofile 655350\n* soft nproc 655350\n* hard nproc 655350\nroot soft nofile 655350\nroot hard nofile 655350\nroot soft nproc 655350\nroot hard nproc 655350\n' | $SUDO tee -a /etc/security/limits.conf >/dev/null 2>/dev/null || true
fi

BBR_OK=$(sysctl net.ipv4.tcp_congestion_control 2>/dev/null | grep -c bbr)
FQ_OK=$(sysctl net.core.default_qdisc 2>/dev/null | grep -c fq)
TFO_OK=$(sysctl net.ipv4.tcp_fastopen 2>/dev/null | awk '{print $3}')

echo ">>> BBR: $([ $BBR_OK -gt 0 ] && echo '✓ 已启用' || echo '✗ 未生效')"
echo ">>> FQ:  $([ $FQ_OK -gt 0 ] && echo '✓ 已启用' || echo '✗ 未生效')"
echo ">>> TFO: $([ "$TFO_OK" = "3" ] && echo '✓ 已启用' || echo "当前值: $TFO_OK")"
echo ">>> 优化完成"
