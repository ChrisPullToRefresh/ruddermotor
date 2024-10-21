
# TODO: rename if needed
bin/custommotor: *.go cmd/module/*.go go.*
	go build -o bin/custommotor cmd/module/cmd.go

bin/remoteserver: *.go cmd/remote/*.go go.*
	go build -o bin/remoteserver cmd/remote/cmd.go

lint:
	gofmt -w -s .

updaterdk:
	go get go.viam.com/rdk@latest
	go mod tidy

# TODO: rename if needed
module: bin/custommotor
	tar czf module.tar.gz bin/custommotor