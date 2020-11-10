FROM alpine:3.12

RUN apk add bash py3-pip git py3-sphinx py3-wheel py3-psycopg2 postgresql build-base python3-dev linux-headers musl-locales \
  && pip3 install 'git+https://github.com/zalando/patroni.git#egg=patroni[kubernetes]' \
  && apk del py3-pip build-base python3-dev linux-headers \
  && rm -fr /var/cache/apk/* /root/.cache /tmp/* \
  && PGHOME=/home/postgres \
  && mkdir -p $PGHOME \
  && chown postgres $PGHOME \
  && sed -i "s|/var/lib/postgresql.*|$PGHOME:/bin/bash|" /etc/passwd \
  && chmod 775 $PGHOME \
  && chmod 664 /etc/passwd

ADD entrypoint.sh /

RUN mkdir /run/postgresql && chown postgres:postgres /run/postgresql \
  && mkdir /var/lock/postgresql && chown postgres:postgres /var/lock/postgresql

EXPOSE 5432 8008
USER postgres
WORKDIR /home/postgres
CMD ["/bin/bash", "/entrypoint.sh"]
