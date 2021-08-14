
.PHONY: build
build: 
	go build ./cmd/reflector.go
	

.PHONY: image
image:
	docker build .
