FROM postgres:12
RUN apt update && apt install patroni -y && rm -rf /var/lib/apt/lists/* \
  && localedef -i tr_TR -c -f UTF-8 -A /usr/share/locale/locale.alias tr_TR.UTF-8 \
  && PGHOME=/home/postgres \
  && mkdir -p $PGHOME \
  && chown postgres $PGHOME \
  && sed -i "s|/var/lib/postgresql.*|$PGHOME:/bin/bash|" /etc/passwd \
  && chmod 775 $PGHOME \
  && chmod 664 /etc/passwd

ADD entrypoint.sh /

RUN mkdir -p /run/postgresql && chown postgres:postgres /run/postgresql \
  && mkdir -p /var/lock/postgresql && chown postgres:postgres /var/lock/postgresql

EXPOSE 5432 8008
USER postgres
WORKDIR /home/postgres
ADD post_init.sh post_init.sh
CMD ["/bin/bash", "/entrypoint.sh"]
