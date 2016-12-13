FROM golang:1.7-alpine

RUN apk update && apk upgrade && \
    apk add --no-cache --virtual .build-deps \
        bash git openssh make openssl && \
    rm -rf /var/cache/apk/*

RUN go get -u github.com/Masterminds/glide/...

COPY . $GOPATH/src/github.com/racker/rackspace-monitoring-poller

WORKDIR $GOPATH/src/github.com/racker/rackspace-monitoring-poller

RUN glide install
RUN go build

RUN openssl req \
    -new \
    -newkey rsa:2048 \
    -nodes \
    -keyout key.pem \
    -x509 \
    -days 365 \
    -out cert.pem \
    -subj "/C=US/ST=Texas/L=Rackspace/O=Dis/CN=www.example.com"

EXPOSE 55000

CMD ./rackspace-monitoring-poller endpoint --config contrib/endpoint-config.json  --debug