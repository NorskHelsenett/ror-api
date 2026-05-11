ARG GCR_MIRROR=gcr.io/
FROM ${GCR_MIRROR}distroless/static:nonroot
LABEL org.opencontainers.image.source=https://github.com/norskhelsenett/ror
WORKDIR /

ARG TARGETARCH
COPY dist/ror-statuspage-linux-${TARGETARCH} /bin/ror-statuspage
EXPOSE 8080
ENTRYPOINT ["/bin/ror-statuspage"]
