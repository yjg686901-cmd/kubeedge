FROM golang:1.25.13-alpine3.23 AS builder

ARG GO_LDFLAGS

WORKDIR /src/kubeedge
COPY . .

RUN CGO_ENABLED=0 go build -mod=vendor -v -o /usr/local/bin/controllermanager -ldflags "$GO_LDFLAGS -w -s" \
    ./cloud/cmd/controllermanager


FROM alpine:3.23

COPY --from=builder /usr/local/bin/controllermanager /usr/local/bin/controllermanager

ENTRYPOINT ["controllermanager"]
