all: test build

test:
	./test.sh

build:
	./build.sh

docker:
	./docker.sh

docker-push:
	./docker.sh push
