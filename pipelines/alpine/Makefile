
build:
	docker pull alpine:latest
	docker save alpine:latest -o alpine.tar.gz
	docker build -t pipeline-alpine:latest .
	docker save pipeline-alpine:latest -o ../pipeline-alpine.tar.gz