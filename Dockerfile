# build stage
FROM golang as builder

ENV GO111MODULE=on

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/kelon github.com/Foundato/kelon/cmd/kelon

# final stage
FROM scratch

ARG PORT=8181

COPY --from=builder /app/kelon /app/

EXPOSE $PORT
ENTRYPOINT ["/app/kelon", "start"]