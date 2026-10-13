# The apic image: the static binary on scratch, with the CA bundle it needs
# for TLS and a world-writable /tmp. goreleaser builds it (dockers_v2 in
# .goreleaser.yaml) for linux/amd64 and linux/arm64, putting each platform's
# binary at $TARGETPLATFORM/apic in the build context.
#
#   docker run --rm -v "$PWD/api:/work" ghcr.io/hungovercoders/apic run smoke.http
#
# The Alpine stage only lays out files, so it runs on the build platform:
# no emulation is needed for the arm64 image.
ARG BASE=alpine:3.22
FROM --platform=$BUILDPLATFORM ${BASE} AS rootfs
RUN mkdir -p /rootfs/etc/ssl/certs /rootfs/tmp \
 && cp /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/ \
 && chmod 1777 /rootfs/tmp

FROM scratch
ARG TARGETPLATFORM
COPY --from=rootfs /rootfs/ /
COPY $TARGETPLATFORM/apic /apic
WORKDIR /work
# The image runs as root on purpose: apic writes .apic/ into the bind-mounted
# project (-v "$PWD:/work"), which a fixed non-root uid could not do on a
# directory the host user owns. Nothing listens; it is a CLI.
# nosemgrep: dockerfile.security.missing-user-entrypoint.missing-user-entrypoint
ENTRYPOINT ["/apic"]
