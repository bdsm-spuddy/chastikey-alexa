SRC:=$(shell echo *.go)
chastikey: $(SRC)
	go build -o $@ -gcflags "all=-trimpath=${GOPATH}" $(SRC)

