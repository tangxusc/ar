build:
	docker build -t containerd-k8s-1_35_0-base:latest .
	docker save containerd-k8s-1_35_0-base:latest -o /tmp/containerd-k8s-1_35_0-base.tar.gz
	mkdir -p /tmp/containerd-k8s-1_35_0-base
	tar -xvf /tmp/containerd-k8s-1_35_0-base.tar.gz -C /tmp/containerd-k8s-1_35_0-base
	rm -rf /tmp/containerd-k8s-1_35_0-base.tar.gz
	cp -r /tmp/containerd-k8s-1_35_0-base /var/lib/ar/images/
	rm -rf /tmp/containerd-k8s-1_35_0-base
	allrun image list