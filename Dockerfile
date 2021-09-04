FROM golang:1.16.7 AS builder

# Add the project
ADD ./go.mod /go/src/github.com/havulv/reflector/
ADD ./go.sum /go/src/github.com/havulv/reflector/
ADD ./cmd/ /go/src/github.com/havulv/reflector/cmd/
ADD ./pkg/ /go/src/github.com/havulv/reflector/pkg/
ADD ./.git /go/src/github.com/havulv/reflector/

RUN set -ex &&  \
  cd /go/src/github.com/havulv/reflector && \
  go mod download

RUN set -ex &&  \
  cd /go/src/github.com/havulv/reflector && \
  CGO_ENABLED=0 go build \
        -tags netgo \
        -v \
        -a \
        -o ./reflector \
		-ldflags "\
          -X $(go list -m)/cmd/version.commitHash=$(git rev-parse --short HEAD) \
		  -X $(go list -m)/cmd/version.semVer=$(git describe --tags --always --dirty) \
		  -X '$(go list -m)/cmd/version.commitDate=$(git log -1 --format=%ci)' \
          -extldflags '-static'" \
         ./cmd/*.go && \
  mv ./reflector /usr/bin/reflector

WORKDIR /go/src/github.com/havulv/reflector/

# Base is required instead of static to do the below adjustments
FROM gcr.io/distroless/base AS prod
COPY --from=builder /usr/bin/reflector /reflector

CMD ["/reflector"]
