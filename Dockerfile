FROM alpine:3.19

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

COPY build/glance-$TARGETOS-$TARGETARCH${TARGETVARIANT} /usr/local/bin/glance

RUN mkdir -p /data
WORKDIR /data

EXPOSE 8080/tcp
ENTRYPOINT ["glance"]
CMD ["--config", "/data/glance.yml"]
