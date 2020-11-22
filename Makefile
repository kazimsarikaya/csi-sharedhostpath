.PHONY: docker

all: test build

test:
	./test.sh $(filter-out $@,$(MAKECMDGOALS))

build:
	./build.sh

docker:
	./docker.sh $(filter-out $@,$(MAKECMDGOALS))

%:
	@:
