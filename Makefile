SRC:=$(shell echo *.go)
chastikey: $(SRC)
	go build -o $@ $(SRC)

