build:
	go build main.go

buildwasm:
	GOOS=js GOARCH=wasm go build -o main.wasm

serve:
	go run server/main.go

wasmserve:
	go run github.com/hajimehoshi/wasmserve@latest ./main.go

clean:
	rm -f main main.wasm