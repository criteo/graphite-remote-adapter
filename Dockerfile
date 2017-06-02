FROM        prom/busybox:latest
MAINTAINER  Corentin Chary <corentin.chary@gmail.com>

COPY graphite-remote-adapter                /bin/graphite-remote-adapter

EXPOSE     9092
VOLUME     [ "/graphite-remote-adapter" ]
WORKDIR    /graphite-remote-adapter
ENTRYPOINT [ "/bin/graphite-remote-adapter" ]
CMD        [ "-graphite-address=localhost:2003" ]
