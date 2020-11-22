.PHONY: docker

all: test build

test:
	./test.sh

test-run:
	./test.sh run

build:
	./build.sh

docker:
	./docker.sh

docker-push:
	./docker.sh push

docker-test:
	./docker.sh test
