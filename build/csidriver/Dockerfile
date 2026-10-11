FROM golang:1.26.9-alpine3.23 AS builder

ARG GO_LDFLAGS

WORKDIR /src/kubeedge
COPY . .
ENV GOTOOLCHAIN=local

RUN CGO_ENABLED=0 go build -mod=vendor -v -o /usr/local/bin/csidriver -ldflags="${GO_LDFLAGS} -w -s" \
./cloud/cmd/csidriver

FROM alpine:3.21

COPY --from=builder /usr/local/bin/csidriver /usr/local/bin/csidriver

ENTRYPOINT ["csidriver"]
