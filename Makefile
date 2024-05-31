build:
	go build main.go
	go build -o server server/main.go

buildwasm:
	GOOS=js GOARCH=wasm go build -o main.wasm
	GOOS=linux GOARCH=amd64 go build -o server server/main.go

serve:
	go run server/main.go

wasmserve:
	go run github.com/hajimehoshi/wasmserve@latest ./main.go

clean:
	rm -f main main.wasm