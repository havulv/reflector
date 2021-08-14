FROM golang:1.16.7 AS builder

# Add the project
ADD ./go.mod /go/src/github.com/havulv/reflector/
ADD ./go.sum /go/src/github.com/havulv/reflector/
ADD ./cmd/ /go/src/github.com/havulv/reflector/cmd/
ADD ./pkg/ /go/src/github.com/havulv/reflector/pkg/

RUN set -ex &&  \
  cd /go/src/github.com/havulv/reflector && \       
  CGO_ENABLED=0 go build \
        -tags netgo \
        -v \
        -a \
        -o ./ \
        -ldflags '-extldflags "-static"' ./... && \
  mv ./reflector /usr/bin/reflector

WORKDIR /go/src/github.com/havulv/reflector/

# Base is required instead of static to do the below adjustments
FROM gcr.io/distroless/base AS prod
COPY --from=builder /usr/bin/reflector /reflector

CMD ["/reflector"]
