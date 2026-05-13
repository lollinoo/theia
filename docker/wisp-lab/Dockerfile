FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      frr \
      iproute2 \
      procps \
      python3 \
      snmp \
      snmpd && \
    rm -rf /var/lib/apt/lists/* && \
    mkdir -p /etc/frr /etc/snmp /opt/wisp-lab /var/log/frr /var/run/frr

COPY docker/wisp-lab/topology.json /opt/wisp-lab/topology.json
COPY docker/wisp-lab/render_router_lab.py /usr/local/bin/render-router-lab.py
COPY docker/wisp-lab/start-router-lab.sh /usr/local/bin/start-router-lab.sh

RUN sed -i 's/\r$//' /usr/local/bin/render-router-lab.py /usr/local/bin/start-router-lab.sh
RUN chmod +x /usr/local/bin/render-router-lab.py /usr/local/bin/start-router-lab.sh

EXPOSE 161/udp

CMD ["/usr/local/bin/start-router-lab.sh"]
