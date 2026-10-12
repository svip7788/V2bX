#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
gray='\033[0;90m'
plain='\033[0m'

repo="svip7788/V2bX-pro"
bin=/usr/local/V2bX/V2bX
conf=/etc/V2bX/config.json

[[ $EUID -ne 0 ]] && echo -e "${red}错误：必须使用 root 用户运行此脚本${plain}" && exit 1

if [[ -f /etc/alpine-release ]] || grep -qi alpine /etc/issue 2>/dev/null; then
    openrc=true
else
    openrc=false
fi

# ---------- 基础 ----------

# 先直连 GitHub，失败走镜像
fetch() {
    local url="https://raw.githubusercontent.com/${repo}/HEAD/$1"
    curl -fsSL --connect-timeout 10 "$url" -o "$2" || curl -fsSL --connect-timeout 15 "https://ghfast.top/${url}" -o "$2"
}

run_remote() {
    local tmp
    tmp=$(mktemp) || return 1
    if ! fetch "$1" "$tmp"; then
        rm -f "$tmp"
        echo -e "${red}下载 $1 失败，请检查本机能否访问 GitHub${plain}"
        return 1
    fi
    shift
    bash "$tmp" "$@"
    local rc=$?
    rm -f "$tmp"
    return $rc
}

svc() {
    if $openrc; then
        case $1 in
            enable) rc-update add V2bX ;;
            disable) rc-update del V2bX ;;
            *) service V2bX "$1" ;;
        esac
    else
        systemctl "$1" V2bX
    fi
}

installed() { [[ -f $bin ]]; }

running() {
    if $openrc; then
        service V2bX status 2>/dev/null | grep -q started
    else
        systemctl is-active --quiet V2bX
    fi
}

enabled() {
    if $openrc; then
        rc-update show 2>/dev/null | grep -q V2bX
    else
        systemctl is-enabled --quiet V2bX 2>/dev/null
    fi
}

need_install() {
    installed && return 0
    echo -e "${red}V2bX 未安装${plain}"
    return 1
}

version() {
    local f v
    for f in /etc/V2bX/.panel_version /usr/local/V2bX/.version; do
        [[ -s $f ]] && v=$(head -n1 "$f") && [[ -n $v ]] && echo "$v" && return
    done
    installed && $bin version 2>/dev/null | awk 'NF {last=$2} END {print last}'
}

pause() {
    [[ -n $cli ]] && return
    echo && read -rp "按回车返回主菜单" _
}

# 等服务状态稳定再判断，避免 Restart=always 的抖动误报
wait_state() {
    local want=$1 i
    for i in 1 2 3 4 5; do
        sleep 1
        if [[ $want == up ]]; then running && return 0; else running || return 0; fi
    done
    return 1
}

# ---------- 服务操作 ----------

start() {
    if running; then
        echo -e "${green}V2bX 已在运行${plain}"
        return
    fi
    svc start
    if wait_state up; then echo -e "${green}V2bX 启动成功${plain}"; else echo -e "${red}V2bX 启动失败，请查看日志${plain}"; fi
}

stop() {
    svc stop
    if wait_state down; then echo -e "${green}V2bX 已停止${plain}"; else echo -e "${red}V2bX 停止失败${plain}"; fi
}

restart() {
    svc restart
    if wait_state up; then echo -e "${green}V2bX 重启成功${plain}"; else echo -e "${red}V2bX 启动失败，请查看日志${plain}"; fi
}

status() {
    if $openrc; then service V2bX status; else systemctl status V2bX --no-pager -l; fi
}

show_log() {
    if $openrc; then
        echo -e "${red}alpine 暂不支持查看日志${plain}"
        return 1
    fi
    echo -e "${gray}Ctrl+C 退出日志${plain}"
    trap ':' INT
    journalctl -u V2bX -n 200 --no-pager -f
    trap - INT
}

toggle_enable() {
    if enabled; then
        svc disable >/dev/null 2>&1 && echo -e "${green}已取消开机自启${plain}"
    else
        svc enable >/dev/null 2>&1 && echo -e "${green}已设置开机自启${plain}"
    fi
}

# ---------- 安装与更新 ----------

do_install() {
    if installed; then
        echo -e "${yellow}V2bX 已安装，如需更新请选择更新${plain}"
        return
    fi
    run_remote install/install.sh "$@"
}

update() {
    local ver=$1
    if [[ -z $cli && -z $ver ]]; then
        read -rp "输入版本号（回车安装最新版）: " ver
    fi
    is_machine && echo -e "${gray}提示：机器模式可直接在面板「升级 V2bX」${plain}"
    run_remote install/install.sh $ver && echo -e "${green}更新完成${plain}"
}

update_shell() {
    local tmp
    tmp=$(mktemp) || return 1
    if fetch install/V2bX.sh "$tmp" && bash -n "$tmp"; then
        install -m 755 "$tmp" /usr/bin/V2bX
        rm -f "$tmp"
        echo -e "${green}管理脚本已更新，请重新运行 V2bX${plain}"
        exit 0
    fi
    rm -f "$tmp"
    echo -e "${red}下载管理脚本失败${plain}"
}

uninstall() {
    read -rp "确定卸载 V2bX？配置文件 /etc/V2bX 将一并删除 [y/N]: " yn
    [[ $yn == [Yy] ]] || return
    svc stop >/dev/null 2>&1
    svc disable >/dev/null 2>&1
    if $openrc; then
        rm -f /etc/init.d/V2bX
    else
        rm -f /etc/systemd/system/V2bX.service
        rm -rf /etc/systemd/system/V2bX.service.d
        systemctl daemon-reload
        systemctl reset-failed 2>/dev/null
    fi
    rm -rf /etc/V2bX /usr/local/V2bX
    echo -e "${green}卸载完成${plain}，如需删除本脚本：rm -f /usr/bin/V2bX /usr/bin/v2bx"
    exit 0
}

# ---------- 配置与工具 ----------

is_machine() { [[ -f $conf ]] && grep -q '"Machines\?"' "$conf"; }

edit_config() {
    is_machine && echo -e "${yellow}机器模式：节点配置由面板下发，这里只有面板对接信息${plain}" && sleep 1
    ${EDITOR:-vi} "$conf"
    read -rp "是否重启 V2bX 使配置生效？[Y/n]: " yn
    [[ $yn == [Nn] ]] || restart
}

# 单节点模式的配置生成向导，与安装时首次配置共用
generate() {
    if is_machine; then
        echo -e "${red}当前为机器模式，配置由面板管理，生成配置会覆盖面板对接信息${plain}"
        read -rp "仍要继续？[y/N]: " yn
        [[ $yn == [Yy] ]] || return
    fi
    local tmp
    tmp=$(mktemp) || return 1
    fetch install/initconfig.sh "$tmp" || { rm -f "$tmp"; echo -e "${red}下载配置向导失败${plain}"; return 1; }
    # shellcheck disable=SC1090
    source "$tmp"
    rm -f "$tmp"
    generate_config_file
}

bbr_on() { [[ $(sysctl -n net.ipv4.tcp_congestion_control 2>/dev/null) == bbr ]]; }

enable_bbr() {
    if bbr_on; then
        echo -e "${green}BBR 已开启${plain}"
        return
    fi
    local tmp
    tmp=$(mktemp) || return 1
    fetch machine/scripts/bbr.sh "$tmp" && sh "$tmp"
    rm -f "$tmp"
}

tune() {
    [[ -f /etc/sysctl.d/99-v2bx-performance.conf ]] || { echo -e "${red}未找到 /etc/sysctl.d/99-v2bx-performance.conf${plain}"; return 1; }
    sysctl --system >/dev/null && echo -e "${green}内核参数已重载${plain}"
}

open_ports() {
    read -rp "将关闭 firewalld/ufw 并清空 iptables 规则，确定？[y/N]: " yn
    [[ $yn == [Yy] ]] || return
    systemctl disable --now firewalld 2>/dev/null
    ufw disable 2>/dev/null
    setenforce 0 2>/dev/null
    iptables -P INPUT ACCEPT 2>/dev/null
    iptables -P FORWARD ACCEPT 2>/dev/null
    iptables -P OUTPUT ACCEPT 2>/dev/null
    iptables -t nat -F 2>/dev/null
    iptables -t mangle -F 2>/dev/null
    iptables -F 2>/dev/null
    iptables -X 2>/dev/null
    netfilter-persistent save 2>/dev/null
    echo -e "${green}已放行全部端口${plain}"
}

x25519() { need_install && $bin x25519; }

pprof() {
    [[ -n $1 ]] || { echo "用法: V2bX pprof <listen_addr> [seconds] [output_dir]"; return 1; }
    [[ -x /usr/local/V2bX/collect_pprof.sh ]] || { echo -e "${red}未找到 collect_pprof.sh${plain}"; return 1; }
    /usr/local/V2bX/collect_pprof.sh "$1" "${2:-20}" "${3:-/tmp/v2bx-pprof-$(date +%Y%m%d-%H%M%S)}"
}

# ---------- 菜单 ----------

uptime_text() {
    local ts s
    ts=$(systemctl show V2bX -p ActiveEnterTimestamp --value 2>/dev/null)
    ts=$(date -d "$ts" +%s 2>/dev/null) || return
    s=$(($(date +%s) - ts))
    if ((s >= 86400)); then
        echo "$((s / 86400)) 天 $((s % 86400 / 3600)) 小时"
    elif ((s >= 3600)); then
        echo "$((s / 3600)) 小时 $((s % 3600 / 60)) 分"
    else
        echo "$((s / 60)) 分钟"
    fi
}

header() {
    local state boot mode bbr since
    if ! installed; then
        state="${red}未安装${plain}"
    elif running; then
        $openrc || since=$(uptime_text)
        state="${green}运行中${plain}${since:+ ${gray}(已运行 ${since})${plain}}"
    else
        state="${red}未运行${plain}"
    fi
    enabled && boot="${green}是${plain}" || boot="${yellow}否${plain}"
    bbr_on && bbr="${green}已开启${plain}" || bbr="${yellow}未开启${plain}（V2bX bbr）"
    if is_machine; then
        mode="机器模式 · $(grep -o '"ApiHost": *"[^"]*"' "$conf" | sed -E 's/.*"https?:\/\/([^"/]+).*/\1/' | sort -u | paste -sd ' ' -)"
    elif [[ -f $conf ]]; then
        mode="单节点模式"
    fi
    echo -e "
  ${green}V2bX 管理脚本${plain}  ${gray}github.com/${repo}${plain}
  版本: ${green}$(version)${plain}${mode:+   ${mode}}
  状态: ${state}   开机自启: ${boot}   BBR: ${bbr}"
}

show_menu() {
    while true; do
        header
        echo -e "  ————————————————————————————
  ${green}1.${plain} 启动      ${green}2.${plain} 停止      ${green}3.${plain} 重启
  ${green}4.${plain} 运行状态  ${green}5.${plain} 实时日志  ${green}6.${plain} 开机自启开关
  ————————————————————————————
  ${green}7.${plain} 更新 V2bX   ${green}8.${plain} 更新管理脚本   ${green}9.${plain} 编辑配置
  ${green}10.${plain} 生成 X25519 密钥   ${green}11.${plain} 放行全部端口
  ————————————————————————————
  ${green}i.${plain} 安装   ${red}u.${plain} 卸载   ${green}0.${plain} 退出"
        echo && read -rp "请选择: " num
        echo
        case $num in
            1) need_install && start ;;
            2) need_install && stop ;;
            3) need_install && restart ;;
            4) need_install && status ;;
            5) need_install && show_log ;;
            6) need_install && toggle_enable ;;
            7) need_install && update ;;
            8) update_shell ;;
            9) need_install && edit_config ;;
            10) x25519 ;;
            11) open_ports ;;
            i | I) do_install ;;
            u | U) need_install && uninstall ;;
            0 | q | "") exit 0 ;;
            *) echo -e "${red}无效选择${plain}" ;;
        esac
        pause
    done
}

usage() {
    echo "用法: V2bX [命令]（不带命令打开菜单，大小写不敏感）
  start | stop | restart | status | log   服务控制
  enable | disable                        开机自启
  update [版本]                           更新 V2bX
  update_shell                            更新管理脚本
  install | uninstall                     安装 / 卸载
  config                                  编辑配置
  generate                                单节点模式配置向导
  x25519                                  生成 X25519 密钥
  bbr                                     开启 BBR
  tune                                    重载内核参数
  pprof <addr> [秒] [目录]                抓取 pprof
  version                                 查看版本"
}

if [[ $# -eq 0 ]]; then
    show_menu
fi

cli=1
cmd=${1,,}
case $cmd in
    start | stop | restart | status) need_install && $cmd ;;
    log) need_install && show_log ;;
    enable | disable) need_install && svc "$cmd" ;;
    update) need_install && update "$2" ;;
    update_shell) update_shell ;;
    install) shift; do_install "$@" ;;
    uninstall) need_install && uninstall ;;
    config) need_install && edit_config ;;
    generate) generate ;;
    x25519) x25519 ;;
    bbr) enable_bbr ;;
    tune) tune ;;
    pprof) shift; pprof "$@" ;;
    version) version ;;
    *) usage ;;
esac
