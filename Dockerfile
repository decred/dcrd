FROM golang:1.12

WORKDIR /go/src/github.com/decred/dcrd
COPY . .

RUN env GO111MODULE=on go install . ./cmd/...

EXPOSE 9108
EXPOSE 9109
EXPOSE 19108
EXPOSE 19109
EXPOSE 18555
EXPOSE 19556

CMD [ "dcrd" ]
