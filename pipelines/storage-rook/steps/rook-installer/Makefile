IMAGE_NAME := rook-installer:latest
IMAGE_STORE_DIR := /var/lib/ar/images/rook-installer
THIS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
PIPELINE_ROOT := $(abspath $(THIS_DIR)/../..)
DOCKERFILE := $(THIS_DIR)/Dockerfile

.PHONY: build offline-images

offline-images:
	mkdir -p "$(THIS_DIR)/ar/images"
	@printf '#!/bin/bash\nset -euo pipefail\n# 此文件由 Makefile offline-images 自动生成，勿手动修改\n' \
		> "$(THIS_DIR)/ar/push-commands.sh"
	@while IFS= read -r image || [ -n "$$image" ]; do \
		[ -z "$$image" ] && continue; \
		archive="$(THIS_DIR)/ar/images/$$(echo "$$image" | tr '/:' '--').tar"; \
		if [ ! -s "$$archive" ]; then \
			echo "Downloading offline image: $$image -> $$archive"; \
			skopeo copy --override-os linux --override-arch amd64 "docker://$$image" "oci-archive:$$archive:$$image"; \
		else \
			echo "Offline image exists, skip: $$archive"; \
		fi; \
		image_path="$$(echo "$$image" | sed 's|^[^/]*/||')"; \
		printf 'skopeo copy --dest-creds "$${REGISTRY_USER}:$${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/%s" "docker://$${REGISTRY}:5000/%s"\n' \
			"$$(basename $$archive)" \
			"$$image_path" \
			>> "$(THIS_DIR)/ar/push-commands.sh"; \
	done < "$(THIS_DIR)/images.txt"
	@chmod +x "$(THIS_DIR)/ar/push-commands.sh"

build:
	$(MAKE) offline-images
	docker build -f "$(DOCKERFILE)" -t $(IMAGE_NAME) .
	rm -rf "$(IMAGE_STORE_DIR)"
	skopeo copy docker-daemon:$(IMAGE_NAME) oci:$(IMAGE_STORE_DIR):latest
	allrun image list
