BINARY := furiwake

.PHONY: build run clean test fmt cross clean-dist checksums release-assets

build:
	go build -o $(BINARY) .

run:
	go run . --config furiwake.yaml

test:
	go test ./...

fmt:
	gofmt -w .
	npx prettier --write .

clean:
	rm -f $(BINARY)
	rm -rf dist

clean-dist:
	rm -rf dist

cross:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe .
	GOOS=windows GOARCH=arm64 go build -o dist/$(BINARY)-windows-arm64.exe .

checksums:
	cd dist && (sha256sum * 2>/dev/null || shasum -a 256 *) > checksums.txt

release-assets: clean-dist cross checksums
