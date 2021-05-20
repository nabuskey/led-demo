ARG GO_VERSION=1.16
FROM golang:${GO_VERSION} as build
WORKDIR /build
ADD . .
RUN CGO_ENABLED=0 GOOS=linux \
    go build -o demo .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt \
     /etc/ssl/certs/ca-certificates.crt
COPY --from=build /build/demo /demo
ENTRYPOINT ["/demo"]