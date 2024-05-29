build:
	go build main.go

serve:
	go run github.com/hajimehoshi/wasmserve@latest ./main.go