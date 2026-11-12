# syntax=docker/dockerfile:1
#
# Test image for casemv. The suites here touch real files, create awkward
# names, and revoke directory permissions, so they run against the container's
# own filesystem rather than the host's. Build context is the repository root.
FROM golang:1.27-alpine@sha256:eb61c3873c97416fff0116553fff65d67a3549f72e11d9758849a689cdd8b523

ENV CGO_ENABLED=0 \
    GOCACHE=/gocache \
    GOMODCACHE=/gomodcache \
    GOFLAGS=-mod=readonly

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /usr/local/bin/casemv . \
 && go test -run=NONE ./... > /dev/null \
 && go test -run=NONE -tags e2e ./test/e2e/... > /dev/null

# Permission tests are meaningless as root, which bypasses the checks they make.
RUN adduser -D -u 1000 tester \
 && mkdir -p /work \
 && chown tester:tester /work \
 && chmod -R a+rwX /gocache /gomodcache /src

USER tester
ENV CASEMV_BIN=/usr/local/bin/casemv \
    TMPDIR=/work

CMD ["sh", "-c", "go test -count=1 ./... && go test -count=1 -tags e2e ./test/e2e/..."]
