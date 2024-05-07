###############
# Builder Stage
###############

# Basic Go environment with git, SSL CA certs, and upx.
# The image below is golang:1.22.3-alpine3.19 (linux/amd64)
# It's pulled by the digest (immutable id) to avoid supply-chain attacks.
# Maintainer Note:
#    To update to a new digest, you must first manually pull the new image:
#    `docker pull golang:<new version>`
#    Docker will print the digest of the new image after the pull has finished.
FROM golang@sha256:2a882244fb51835ebbd8313bffee83775b0c076aaf56b497b43d8a4c72db65e1 AS builder
RUN apk add --no-cache git ca-certificates upx

# Empty directory to be copied into place in the production image since it will
# run as a non-root container and thus not have permissions to create
# directories or change ownership of anything outside of the structure already
# created for it.
RUN mkdir /emptydatadir

# New unprivileged user for use in production image below to improve security.
ENV USER=decred
ENV UID=10000
RUN adduser \
    --disabled-password \
    --gecos "" \
    --home="/home/${USER}" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    "${USER}"

# Build dcrd and other commands it provides.
WORKDIR /go/src/github.com/decred/dcrd
ARG DCRD_BUILD_TAG=master
RUN git clone --branch ${DCRD_BUILD_TAG} -c advice.detachedHead=false https://github.com/decred/dcrd . && \
    CGO_ENABLED=0 GOOS=linux \
    go install -trimpath -tags safe,netgo,timetzdata \
      -ldflags="-s -w" \
      . ./cmd/gencerts ./cmd/promptsecret

# Build dcrctl.
WORKDIR /go/src/github.com/decred/dcrctl
ARG DCRCTL_BUILD_TAG=${DCRD_BUILD_TAG}
RUN git clone --branch ${DCRCTL_BUILD_TAG} -c advice.detachedHead=false https://github.com/decred/dcrctl . && \
    CGO_ENABLED=0 GOOS=linux \
    go install -trimpath -tags safe,netgo -ldflags="-s -w"

# Build entrypoint helper for the production image.
WORKDIR /go/src/github.com/decred/dcrd/contrib/docker/entrypoint
COPY ./contrib/docker/entrypoint/entrypoint.go .
RUN go mod init entrypoint && \
    go mod tidy && \
    CGO_ENABLED=0 GOOS=linux \
    go install -trimpath -tags netgo,timetzdata -ldflags="-s -w" .

# Compress bins.
RUN upx -9 /go/bin/*

##################
# Production image
##################

# Minimal scratch-based environment.
FROM scratch
ENV DECRED_DATA=/home/decred
ENV DCRD_NO_FILE_LOGGING=true
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/* /bin/
COPY --from=builder --chown=decred /emptydatadir /tmp

# Use an unprivileged user.
USER decred

# Ports for the p2p and json-rpc of mainnet, testnet, and simnet, respectively.
EXPOSE 9108 9109 19108 19109 18555 19556

ENTRYPOINT [ "/bin/entrypoint" ]

RUN [ "dcrd", "--version" ]
RUN [ "dcrctl", "--version" ]

# The volume instruction is intentionally commented here since it has many
# well-known pitfalls.  It is only provided here for documentation purposes.
#
# The preferred approach to mounting the volume is to specify the binding via
# the `-v` flag of `docker run` or via a Docker Compose file.
#VOLUME [ "/home/decred" ]
