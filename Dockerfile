FROM golang:1.14

#
# NOTE: The RPC server listens on localhost by default.
#       If you require access to the RPC server,
#       rpclisten should be set to an empty value.
#
# NOTE: When running simnet, you may not want to preserve
#       the data and logs.  This can be achieved by specifying
#       a location outside the default ~/.dcrd.  For example:
#          rpclisten=
#          simnet=1
#          datadir=~/simnet-data
#          logdir=~/simnet-logs
#
# Example testnet instance with RPC server access:
# $ mkdir -p /local/path/dcrd
#
# Place a dcrd.conf into a local directory, i.e. /var/dcrd
# $ mv dcrd.conf /var/dcrd
#
# Verify basic configuration
# $ cat /var/dcrd/dcrd.conf
# rpclisten=
# testnet=1
#
# Build the docker image
# $ docker build -t user/dcrd .
#
# Run the docker image, mapping the testnet dcrd RPC port.
# $ docker run -d --rm -p 127.0.0.1:19109:19109 -v /var/dcrd:/root/.dcrd user/dcrd
#

WORKDIR /go/src/github.com/decred/dcrd
COPY . .

RUN env GO111MODULE=on go install . ./cmd/...

# mainnet
EXPOSE 9108 9109

# testnet
EXPOSE 19108 19109

# simnet
EXPOSE 18555 19556

CMD [ "dcrd" ]
