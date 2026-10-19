# The apic image: the static binary on scratch, with the CA bundle it needs
# for TLS and a world-writable /tmp. goreleaser builds it (dockers_v2 in
# .goreleaser.yaml) for linux/amd64 and linux/arm64, putting each platform's
# binary at $TARGETPLATFORM/apic in the build context, with LICENSE and
# THIRD_PARTY_NOTICES.md beside it.
#
#   docker run --rm -v "$PWD/api:/work" ghcr.io/hungovercoders/apic run smoke.http
#
# The image is a distribution of apic and of everything compiled into it, so
# it carries the same licence and notices as the archives, in
# /usr/share/doc/apic. The CA bundle is Mozilla's certificate store (MPL-2.0)
# as Alpine packages it; CA-CERTIFICATES.md names the package and version it
# came from and where its source is.
#
# The Alpine stage only lays out files, so it runs on the build platform:
# no emulation is needed for the arm64 image.
ARG BASE=alpine:3.22
FROM --platform=$BUILDPLATFORM ${BASE} AS rootfs
RUN mkdir -p /rootfs/etc/ssl/certs /rootfs/tmp /rootfs/usr/share/doc/apic \
 && cp /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/ \
 && chmod 1777 /rootfs/tmp \
 && pkg=$(apk list --installed ca-certificates-bundle) \
 && test -n "$pkg" \
 && printf '%s\n' \
      '# CA certificates' \
      '' \
      '/etc/ssl/certs/ca-certificates.crt is copied from this Alpine Linux package:' \
      '' \
      "    $pkg" \
      '' \
      "The certificates are Mozilla's CA certificate store, under the Mozilla" \
      'Public License 2.0: https://mozilla.org/MPL/2.0/' \
      '' \
      'Source: https://gitlab.alpinelinux.org/alpine/aports/-/tree/master/main/ca-certificates' \
      'and https://hg.mozilla.org/projects/nss/raw-file/tip/lib/ckfw/builtins/certdata.txt' \
      > /rootfs/usr/share/doc/apic/CA-CERTIFICATES.md

FROM scratch
ARG TARGETPLATFORM
COPY --from=rootfs /rootfs/ /
COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/apic/
COPY $TARGETPLATFORM/apic /apic
WORKDIR /work
# The image runs as root on purpose: apic writes .apic/ into the bind-mounted
# project (-v "$PWD:/work"), which a fixed non-root uid could not do on a
# directory the host user owns. Nothing listens; it is a CLI.
# nosemgrep: dockerfile.security.missing-user-entrypoint.missing-user-entrypoint
ENTRYPOINT ["/apic"]
