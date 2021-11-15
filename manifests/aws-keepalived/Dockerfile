FROM amazon/aws-cli:2.1.29

RUN yum update && yum install -y keepalived procps sysvinit-tools

ENTRYPOINT ["/usr/bin/startup.sh"]

COPY keepalived.conf.tmpl /etc/keepalived/keepalived.conf

COPY notify-master-script.sh /etc/keepalived/notify-master-script.sh

COPY notify-backup-script.sh /etc/keepalived/notify-backup-script.sh

COPY check-script.sh /etc/keepalived/check-script.sh

COPY startup.sh /usr/bin/startup.sh
