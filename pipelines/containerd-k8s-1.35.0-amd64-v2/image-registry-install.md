# 镜像仓库安装

在containerd安装好之后,需要安装镜像仓库,用以在内网环境各节点拉取镜像
在makefile build中增加命令,用于使用skopeo 下载镜像到steps/base/ar/images目录下,名称为镜像名称加版本号，格式为oci
在前序步骤中,这个文件会被拷贝至目标节点的/tmp/ar/images目录下

1. 根据 节点标签为registry=true的节点,渲染/ar/confs/image-registry/hosts.toml,渲染后存储在/ar-data/confs/registry/hosts.toml
2. 复制/ar-data/confs/registry/hosts.toml到所有节点的/tmp/ar/confs/image-registry/hosts.toml

在节点标签为registry=true的节点上执行
3. 在scripts目录下新增image-registry-install.sh脚本,用于在目标节点安装镜像仓库
3.1 使用containerd命令导入镜像仓库的镜像到本地containerd
3.2 复制/tmp/ar/confs/image-registry/* 到 /etc/registry/
3.3 启动registry服务,端口5000,不使用https协议,restart=always,配置文件使用 -v 映射/etc/registry/config.yml,认证文件(htpasswd)使用 -v 映射/etc/registry/htpasswd,数据目录使用 -v 映射/var/lib/registry

在所有节点上执行
4.1 复制/tmp/ar/confs/image-registry/hosts.toml 到 /etc/containerd/cert.d/_default/hosts.html
4.2 重启containerd服务