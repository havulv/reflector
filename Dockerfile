FROM golang:1.21.5 AS builder

ARG COMMIT_HASH
ARG SEMVER
ARG COMMIT_DATE

# Add the project module dependencies
ADD ./go.mod /go/src/github.com/havulv/reflector/
ADD ./go.sum /go/src/github.com/havulv/reflector/

RUN set -ex &&  \
  cd /go/src/github.com/havulv/reflector && \
  go mod download

# Add the project afterwards to avoid non dependencies blowing
# up the docker cache
ADD ./cmd/ /go/src/github.com/havulv/reflector/cmd/
ADD ./pkg/ /go/src/github.com/havulv/reflector/pkg/

RUN set -ex &&  \
  cd /go/src/github.com/havulv/reflector && \
  CGO_ENABLED=0 go build \
        -tags netgo \
        -v \
        -a \
        -o ./reflector \
		-ldflags "\
          -w \
          -X $(go list -m)/cmd/version.commitHash=\"$COMMIT_HASH\" \
		  -X $(go list -m)/cmd/version.semVer=\"$SEMVER\" \
		  -X '$(go list -m)/cmd/version.commitDate=\"$COMMIT_DATE\"' \
          -extldflags '-static'" \
         ./cmd/*.go && \
  mv ./reflector /usr/bin/reflector

WORKDIR /go/src/github.com/havulv/reflector/

# Base is required instead of static to do the below adjustments
FROM gcr.io/distroless/base AS prod
COPY --from=builder /usr/bin/reflector /reflector

CMD ["/reflector"]
