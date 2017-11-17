FROM decred/decred-golang-builder-1.9 as builder
COPY . /go/src/github.com/decred/dcrd
WORKDIR /go/src/github.com/decred/dcrd
RUN dep ensure
ENV CGO_ENABLED=0
RUN mkdir -p /tmp/output
RUN go build -ldflags "-s -w" -a -tags netgo -o /tmp/output/dcrd .
WORKDIR /go/src/github.com/decred/dcrd/cmd
RUN for cmd in `echo *`; do go build -ldflags "-s -w" -a -tags netgo -o /tmp/output/$cmd ./$cmd; done

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /tmp/output/* /usr/local/bin/
VOLUME /var/lib/dcrd
VOLUME /etc/dcrd
EXPOSE 9108
EXPOSE 9109
ENTRYPOINT [ "/usr/local/bin/dcrd", "--datadir=/var/lib/dcrd", "--nofilelogging", "--configfile=/etc/dcrd/config", "--rpccert=/etc/dcrd/rpc.cert", "--rpckey=rpc.key" ]