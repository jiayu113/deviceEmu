FROM safarov/freeswitch

COPY gen-users.sh /gen-users.sh
RUN sh /gen-users.sh 1000 1119

RUN sed -i 's|external_rtp_ip=stun:stun.freeswitch.org|external_rtp_ip=192.168.1.10|' /usr/share/freeswitch/conf/vanilla/vars.xml
RUN sed -i 's|external_sip_ip=stun:stun.freeswitch.org|external_sip_ip=192.168.1.10|' /usr/share/freeswitch/conf/vanilla/vars.xml