#!/usr/bin/env python3
import ipaddress
import json
import os
from pathlib import Path


TOPOLOGY_PATH = Path(os.environ.get("TOPOLOGY_PATH", "/opt/wisp-lab/topology.json"))
SNMP_OUTPUT_PATH = Path(os.environ.get("SNMP_OUTPUT_PATH", "/etc/snmp/snmpd.conf"))
FRR_OUTPUT_PATH = Path(os.environ.get("FRR_OUTPUT_PATH", "/etc/frr/frr.conf"))
DAEMONS_OUTPUT_PATH = Path(os.environ.get("DAEMONS_OUTPUT_PATH", "/etc/frr/daemons"))
VTYSH_OUTPUT_PATH = Path(os.environ.get("VTYSH_OUTPUT_PATH", "/etc/frr/vtysh.conf"))
RUNTIME_ENV_PATH = Path(os.environ.get("RUNTIME_ENV_PATH", "/run/wisp-router.env"))

DEFAULT_SYS_OBJECT_ID = "1.3.6.1.4.1.14988.1"
LOCAL_PORT_IFINDEX_OID = "1.0.8802.1.1.2.1.3.7.1.3"
LOCAL_PORT_ID_OID = "1.0.8802.1.1.2.1.3.7.1.4"
REM_CHASSIS_SUBTYPE_OID = "1.0.8802.1.1.2.1.4.1.1.4"
REM_CHASSIS_OID = "1.0.8802.1.1.2.1.4.1.1.5"
REM_PORT_SUBTYPE_OID = "1.0.8802.1.1.2.1.4.1.1.6"
REM_PORT_OID = "1.0.8802.1.1.2.1.4.1.1.7"
REM_PORT_DESC_OID = "1.0.8802.1.1.2.1.4.1.1.8"
REM_SYS_NAME_OID = "1.0.8802.1.1.2.1.4.1.1.9"
REM_SYS_DESC_OID = "1.0.8802.1.1.2.1.4.1.1.10"


def all_devices(topology: dict) -> dict[str, dict]:
    devices: dict[str, dict] = {}
    devices.update(topology.get("routers", {}))
    devices.update(topology.get("access_devices", {}))
    return devices


def load_topology() -> tuple[dict, dict]:
    topology = json.loads(TOPOLOGY_PATH.read_text())
    devices = all_devices(topology)
    router_name = os.environ["ROUTER_NAME"]
    if router_name not in devices:
        raise SystemExit(f"device {router_name!r} not found in {TOPOLOGY_PATH}")
    return topology, devices[router_name]


def router_mac(device: dict, ifindex: int) -> str:
    rid = int(device["id"])
    return f"02:42:11:{rid:02x}:{ifindex:02x}:01"


def sys_descr(device: dict) -> str:
    if device.get("sys_descr"):
        return device["sys_descr"]
    return f'RouterOS {device["model"]} 7.15.3 (stable)'


def sys_object_id(device: dict) -> str:
    return device.get("sys_object_id", DEFAULT_SYS_OBJECT_ID)


def sys_contact(device: dict) -> str:
    return device.get("contact", "noc@theia.lab")


def sys_services(device: dict) -> int:
    return int(device.get("sys_services", 78))


def iface_if_type(iface: dict) -> int:
    return int(iface.get("if_type", 6))


def iface_descr(iface: dict) -> str:
    return iface.get("if_descr", iface["name"])


def iface_neighbors(iface: dict) -> list[dict]:
    if "neighbors" in iface:
        return list(iface["neighbors"])
    if "peer" in iface:
        return [{"peer": iface["peer"], "peer_if": iface["peer_if"]}]
    return []


def render_snmp(topology: dict, router: dict) -> str:
    devices = all_devices(topology)
    interfaces = router["interfaces"]
    lines = [
        "rocommunity public",
        "agentAddress udp:161",
        "",
        f'override 1.3.6.1.2.1.1.1.0 octet_str "{sys_descr(router)}"',
        f"override 1.3.6.1.2.1.1.2.0 object_id {sys_object_id(router)}",
        "override 1.3.6.1.2.1.1.3.0 integer 8640000",
        f'override 1.3.6.1.2.1.1.4.0 octet_str "{sys_contact(router)}"',
        f'override 1.3.6.1.2.1.1.5.0 octet_str "{router["sys_name"]}"',
        f'override 1.3.6.1.2.1.1.6.0 octet_str "{router["location"]}"',
        f"override 1.3.6.1.2.1.1.7.0 integer {sys_services(router)}",
        "",
        f"override 1.3.6.1.2.1.2.1.0 integer {len(interfaces)}",
        "",
    ]

    for iface in interfaces:
        idx = iface["ifindex"]
        speed_bps = iface["speed_mbps"] * 1_000_000
        if_speed = min(speed_bps, 4_294_967_295)
        lines.extend([
            f"override 1.3.6.1.2.1.2.2.1.1.{idx} integer {idx}",
            f'override 1.3.6.1.2.1.2.2.1.2.{idx} octet_str "{iface_descr(iface)}"',
            f"override 1.3.6.1.2.1.2.2.1.3.{idx} integer {iface_if_type(iface)}",
            f"override 1.3.6.1.2.1.2.2.1.4.{idx} integer 1500",
            f"override 1.3.6.1.2.1.2.2.1.5.{idx} gauge32 {if_speed}",
            f'override 1.3.6.1.2.1.2.2.1.6.{idx} octet_str "{router_mac(router, idx)}"',
            f"override 1.3.6.1.2.1.2.2.1.7.{idx} integer 1",
            f"override 1.3.6.1.2.1.2.2.1.8.{idx} integer 1",
            f'override 1.3.6.1.2.1.31.1.1.1.1.{idx} octet_str "{iface["name"]}"',
            f"override 1.3.6.1.2.1.31.1.1.1.15.{idx} gauge32 {iface['speed_mbps']}",
            f"override {LOCAL_PORT_IFINDEX_OID}.{idx} integer {idx}",
            f'override {LOCAL_PORT_ID_OID}.{idx} octet_str "{iface["name"]}"',
            "",
        ])

    for iface in interfaces:
        idx = iface["ifindex"]
        for neighbor_index, neighbor in enumerate(iface_neighbors(iface), start=1):
            peer = devices[neighbor["peer"]]
            remote_if = neighbor["peer_if"]
            index_suffix = f"0.{idx}.{neighbor_index}"
            lines.extend([
                f"override {REM_CHASSIS_SUBTYPE_OID}.{index_suffix} integer 4",
                f'override {REM_CHASSIS_OID}.{index_suffix} octet_str "{router_mac(peer, 1)}"',
                f"override {REM_PORT_SUBTYPE_OID}.{index_suffix} integer 5",
                f'override {REM_PORT_OID}.{index_suffix} octet_str "{remote_if}"',
                f'override {REM_PORT_DESC_OID}.{index_suffix} octet_str "{remote_if}"',
                f'override {REM_SYS_NAME_OID}.{index_suffix} octet_str "{peer["sys_name"]}"',
                f'override {REM_SYS_DESC_OID}.{index_suffix} octet_str "{sys_descr(peer)}"',
                "",
            ])

    return "\n".join(lines).rstrip() + "\n"


def unique_ospf_networks(router: dict) -> list[tuple[str, str]]:
    seen: set[tuple[str, str]] = set()
    networks: list[tuple[str, str]] = []
    for iface in router["interfaces"]:
        area = iface.get("area")
        if not area:
            continue
        key = (iface["network"], iface["area"])
        if key not in seen:
            seen.add(key)
            networks.append(key)
    loopback = router.get("loopback", {})
    loopback_area = loopback.get("area")
    loopback_cidr = loopback.get("cidr")
    if loopback_area and loopback_cidr:
        loop_key = (loopback_cidr, loopback_area)
        if loop_key not in seen:
            networks.append(loop_key)
    return networks


def render_frr(router: dict) -> str:
    loopback_ip = ipaddress.ip_interface(router["loopback"]["cidr"]).ip
    lines = [
        "frr defaults traditional",
        f'hostname {router["sys_name"]}',
        "service integrated-vtysh-config",
        "log stdout",
        "!",
    ]

    ospf_networks = unique_ospf_networks(router)
    if ospf_networks:
        lines.extend([
            "router ospf",
            f" ospf router-id {loopback_ip}",
        ])
        for network, area in ospf_networks:
            lines.append(f" network {network} area {area}")

        default_mode = router.get("originate_default_mode")
        if default_mode == "conditional":
            lines.append(" default-information originate metric 10 metric-type 1")
        elif default_mode == "always" or router.get("originate_default"):
            lines.append(" default-information originate always metric 10 metric-type 1")
        lines.append("!")

    bgp = router.get("bgp")
    if bgp:
        lines.extend([
            f'router bgp {bgp["asn"]}',
            f" bgp router-id {loopback_ip}",
            " no bgp ebgp-requires-policy",
            " bgp log-neighbor-changes",
        ])
        for neighbor in bgp.get("neighbors", []):
            lines.append(f' neighbor {neighbor["address"]} remote-as {neighbor["remote_as"]}')
            description = neighbor.get("description")
            if description:
                lines.append(f' neighbor {neighbor["address"]} description {description}')
        lines.extend([
            " !",
            " address-family ipv4 unicast",
        ])
        for neighbor in bgp.get("neighbors", []):
            lines.append(f'  neighbor {neighbor["address"]} activate')
            if neighbor.get("default_originate"):
                lines.append(f'  neighbor {neighbor["address"]} default-originate')
            if neighbor.get("next_hop_self"):
                lines.append(f'  neighbor {neighbor["address"]} next-hop-self')
        for network in bgp.get("networks", []):
            lines.append(f"  network {network}")
        lines.extend([
            " exit-address-family",
            "!",
        ])

    lines.extend([
        "line vty",
        "!",
    ])
    return "\n".join(lines) + "\n"


def render_daemons(router: dict) -> str:
    ospfd_enabled = "yes" if unique_ospf_networks(router) else "no"
    bgpd_enabled = "yes" if router.get("bgp") else "no"
    return f"""zebra=yes
bgpd={bgpd_enabled}
ospfd={ospfd_enabled}
ospf6d=no
ripd=no
ripngd=no
isisd=no
pimd=no
pim6d=no
ldpd=no
nhrpd=no
eigrpd=no
babeld=no
sharpd=no
staticd=no
pbrd=no
bfdd=no
fabricd=no
pathd=no
vrrpd=no
watchfrr=yes

vtysh_enable=yes
zebra_options=" -A 127.0.0.1"
bgpd_options=" -A 127.0.0.1"
ospfd_options=" -A 127.0.0.1"
watchfrr_options=""
"""


def render_runtime_env(router: dict) -> str:
    loopback_cidr = router.get("loopback", {}).get("cidr", "")
    enable_ospfd = "1" if unique_ospf_networks(router) else "0"
    enable_bgpd = "1" if router.get("bgp") else "0"
    return (
        f'LOOPBACK_CIDR="{loopback_cidr}"\n'
        f'ENABLE_OSPFD="{enable_ospfd}"\n'
        f'ENABLE_BGPD="{enable_bgpd}"\n'
    )


def write_outputs(topology: dict, router: dict) -> None:
    for path in [SNMP_OUTPUT_PATH, FRR_OUTPUT_PATH, DAEMONS_OUTPUT_PATH, VTYSH_OUTPUT_PATH, RUNTIME_ENV_PATH]:
        path.parent.mkdir(parents=True, exist_ok=True)

    SNMP_OUTPUT_PATH.write_text(render_snmp(topology, router))
    FRR_OUTPUT_PATH.write_text(render_frr(router))
    DAEMONS_OUTPUT_PATH.write_text(render_daemons(router))
    VTYSH_OUTPUT_PATH.write_text("service integrated-vtysh-config\n")
    RUNTIME_ENV_PATH.write_text(render_runtime_env(router))


def main() -> None:
    topology, router = load_topology()
    write_outputs(topology, router)


if __name__ == "__main__":
    main()
