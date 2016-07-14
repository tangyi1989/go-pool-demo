GO = go
ENV = export GOPATH=`pwd`/lib:`pwd`; export GOROOT=$(GOROOT);
GOGET = export GOPATH=`pwd`/lib; $(GO) get
BUILD = $(ENV) $(GO) build

dep:
	$(GOGET) gopkg.in/fatih/pool.v2

build: dep
	$(BUILD) -o ./bin/sock-pool ./src/

run:
	./bin/sock-pool
