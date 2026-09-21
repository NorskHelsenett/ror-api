ARG GCR_MIRROR=gcr.io/
FROM ${GCR_MIRROR}distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
LABEL org.opencontainers.image.source=https://github.com/norskhelsenett/ror-api
WORKDIR /

ARG TARGETARCH
COPY dist/ror-api-linux-${TARGETARCH} /bin/ror-api
EXPOSE 8080
ENTRYPOINT ["/bin/ror-api"]
