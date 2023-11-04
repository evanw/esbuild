FROM alpine:3

WORKDIR /tmp
COPY version.txt .
RUN addgroup -g 1000 -S app && adduser -u 1000 -S app -G app \
 && wget -c https://registry.npmjs.org/@esbuild/linux-x64/-/linux-x64-$(cat version.txt).tgz -O - | tar -xz

FROM scratch

COPY --from=0 /etc/passwd /etc/group /etc/
COPY --from=0 --chown=1000:1000 /tmp/package/bin/esbuild /bin/
USER app

LABEL org.opencontainers.image.authors="Evan Wallace"
LABEL org.opencontainers.image.base.name="docker.io/library/scratch"
LABEL org.opencontainers.image.description="An extremely fast bundler for the web"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.source="https://github.com/evanw/esbuild"
LABEL org.opencontainers.image.title="esbuild"
LABEL org.opencontainers.image.version="0.19.5"

ENTRYPOINT ["/bin/esbuild"]
