# Pinned to the go.mod toolchain patch level so the image build never
# depends on a GOTOOLCHAIN auto-download.
FROM --platform=$BUILDPLATFORM golang:1.25.7-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-s -w -X main.version=${VERSION}" -o /nimsforestmastery ./cmd/nimsforestmastery

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /nimsforestmastery /usr/local/bin/nimsforestmastery
# The image carries the shared doctrine curriculum as an on-disk COPY,
# never a go:embed. Role content is NOT in the image: role pages and
# bundles render at request time from the org's own Soil.
COPY content /usr/share/nimsforestmastery/content
ENV CONTENT_DIR=/usr/share/nimsforestmastery/content
RUN adduser -D -H nimsforest
USER nimsforest
EXPOSE 8110
ENTRYPOINT ["nimsforestmastery"]
