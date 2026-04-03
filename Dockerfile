ARG GCR_MIRROR=gcr.io/
FROM ${GCR_MIRROR}distroless/static:nonroot
LABEL org.opencontainers.image.source=https://github.com/norskhelsenett/ror
WORKDIR /

ARG TARGETARCH
COPY dist/ror-api-linux-${TARGETARCH} /bin/ror-api
EXPOSE 8080
ENTRYPOINT ["/bin/ror-api"]
