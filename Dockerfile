# build stage
FROM golang as builder
# Add dependencies
WORKDIR /go/src/app
ADD . /go/src/app
# Build app
RUN go mod download
RUN go build -o /go/bin/app github.com/Foundato/kelon/cmd/kelon

# final stage
FROM gcr.io/distroless/base
ARG PORT=8181

COPY --from=builder /go/bin/app /
EXPOSE $PORT
ENTRYPOINT ["/app", "start"]