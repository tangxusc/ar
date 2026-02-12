FROM scratch

COPY entrypoint.sh /entrypoint.sh
COPY alpine.template.json /alpine.template.json
COPY alpine.tar.gz /alpine.tar.gz

ENTRYPOINT ["/entrypoint.sh"]