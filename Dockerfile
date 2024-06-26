FROM golang:1.20 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN GOOS=js GOARCH=wasm go build -o main.wasm main.go
RUN GOOS=linux GOARCH=amd64 go build -o server/main server/main.go

FROM debian:bullseye-slim

COPY --from=builder /app/server/main ./server/main
COPY --from=builder /app/main.wasm ./main.wasm
COPY --from=builder /app/wasm_exec.js ./wasm_exec.js
COPY --from=builder /app/index.html ./index.html
COPY --from=builder /app/assets ./assets

EXPOSE 8080

CMD ["./server/main"]