FROM --platform=$BUILDPLATFORM golang:1.17@sha256:87262e4a4c7db56158a80a18fefdc4fee5accc41b59cde821e691d05541bbb18

ARG BUILDPLATFORM
ARG TARGETARCH=amd64
ARG TARGETOS=linux

ENV GO111MODULE=on
WORKDIR /go/src/github.com/wish/nodetaint

# Cache dependencies
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . /go/src/github.com/wish/nodetaint/

RUN go mod tidy
# Build controller
RUN CGO_ENABLED=0 GOARCH=${TARGETARCH} GOOS=${TARGETOS} go build -o . -a -installsuffix cgo .

FROM alpine:3.15@sha256:19b4bcc4f60e99dd5ebdca0cbce22c503bbcff197549d7e19dab4f22254dc864
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=0 /go/src/github.com/wish/nodetaint/nodetaint /root/nodetaint
ENTRYPOINT /root/nodetaint
