build:
	cp *.template.json /var/lib/ar/pipelines/
	@for dir in steps/*; do \
		if [ -d "$$dir" ]; then \
			(cd $$dir && make build); \
		fi; \
	done
	allrun pipeline list
test:
	allrun pipeline run -p containerd-k8s-1_35_0-amd64 -n /home/ubuntu/ar/backend/test/node.json