# Build image
FROM golang:1.11

WORKDIR /go/src/github.com/decred/dcrd
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GO111MODULE=on go install . ./cmd/...

# Production image
FROM alpine:3.6

RUN apk add --no-cache ca-certificates
COPY --from=0 /go/bin/* /bin/

EXPOSE 9108

CMD [ "dcrd" ]
