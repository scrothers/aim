# Container image for the AIM daemon (the hub). Built by GoReleaser
# (dockers_v2 in .goreleaser.yaml), which places the prebuilt static
# binary at $TARGETPLATFORM/aim in the build context; nothing is compiled
# here. Agents still need the aim binary on their own machines for
# `aim mcp` and `aim hook`; they reach this container over TCP.

# /data must exist and belong to the nonroot user so a fresh named volume
# inherits that ownership. Distroless has no shell, so make it in a
# throwaway stage. It runs on the build platform: no emulation needed.
FROM --platform=$BUILDPLATFORM busybox:1.38@sha256:a46257e06c82ad85242d0d38077d0a52d9896389ca60a1a59d3463db586e5fa4 AS data
RUN mkdir -p /data/backups && chown -R 65532:65532 /data && chmod 0700 /data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG TARGETPLATFORM
COPY --from=data --chown=65532:65532 /data /data
COPY $TARGETPLATFORM/aim /usr/bin/aim

# No unix socket in a container; agents connect to the TCP listener.
# The MCP listener and the console/API have no authentication: publish
# them only on networks you trust (see docs/operations.md).
ENV AIM_DB=/data/aim.db \
    AIM_BACKUP_DIR=/data/backups \
    AIM_SOCKET= \
    AIM_LISTEN=:7171 \
    AIM_HTTP=:7172

VOLUME ["/data"]
EXPOSE 7171 7172
USER 65532:65532
ENTRYPOINT ["/usr/bin/aim"]
CMD ["daemon"]
