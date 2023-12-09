# build stage
FROM golang as builder

ARG CGO_ENABLED=0

# Add dependencies
WORKDIR /go/src/app
ADD . /go/src/app
# Build app
RUN go mod download
RUN go build -o /go/bin/app github.com/unbasical/kelon/cmd/kelon

# final stage
FROM gcr.io/distroless/base as build
ARG PORT=8181

COPY --from=builder /go/bin/app /
EXPOSE $PORT
ENTRYPOINT ["/app", "run"]